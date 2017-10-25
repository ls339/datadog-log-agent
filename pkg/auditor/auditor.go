// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package auditor

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/message"
)

const defaultFlushPeriod = 1 * time.Second
const defaultCleanupPeriod = 300 * time.Second
const defaultTTL = 23 * time.Hour

// A RegistryEntry represends an entry in the registry where we keep track
// of current offsets
type RegistryEntry struct {
	Path      string
	Timestamp time.Time
	Offset    int64
}

// An Auditor handles messages successfully submitted to the intake
type Auditor struct {
	inputChan     chan message.Message
	registry      map[string]*RegistryEntry
	registryMutex *sync.Mutex
	registryPath  string

	flushTicker   *time.Ticker
	flushPeriod   time.Duration
	cleanupTicker *time.Ticker
	cleanupPeriod time.Duration
	entryTTL      time.Duration
}

// New returns an initialized Auditor
func New(inputChan chan message.Message) *Auditor {
	return &Auditor{
		inputChan:     inputChan,
		registryPath:  filepath.Join(config.LogsAgent.GetString("run_path"), "registry.json"),
		registryMutex: &sync.Mutex{},

		flushPeriod:   defaultFlushPeriod,
		cleanupPeriod: defaultCleanupPeriod,
		entryTTL:      defaultTTL,
	}
}

// Start starts the Auditor
func (a *Auditor) Start() {
	a.registry = a.recoverRegistry(a.registryPath)
	a.cleanupRegistry(a.registry)
	go a.run()
	go a.flushRegistryPediodically()
	go a.cleanupRegistryPeriodically()
}

// flushRegistryPediodically periodically saves the registry in its current state
func (a *Auditor) flushRegistryPediodically() {
	a.flushTicker = time.NewTicker(a.flushPeriod)
	for {
		select {
		case <-a.flushTicker.C:
			err := a.flushRegistry(a.registry, a.registryPath)
			if err != nil {
				log.Println(err)
			}
		}
	}
}

// cleanupRegistryPeriodically periodically removes from the registry expired offsets
func (a *Auditor) cleanupRegistryPeriodically() {
	a.cleanupTicker = time.NewTicker(a.cleanupPeriod)
	for {
		select {
		case <-a.cleanupTicker.C:
			a.cleanupRegistry(a.registry)
		}
	}
}

// run lets the auditor update the registry
func (a *Auditor) run() {
	for msg := range a.inputChan {
		// An offset of 0 means that we don't want to store the offset for that origin.
		// This is useful for origins that don't have offsets (networks), or when we
		// specially want to avoid storing the offset
		if msg.GetOrigin().Offset > 0 {
			a.updateRegistry(msg.GetOrigin().LogSource.Path, msg.GetOrigin().Offset)
		}
	}
}

// updateRegistry updates the offset of path in the auditor's registry
func (a *Auditor) updateRegistry(path string, offset int64) {
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	entry, ok := a.registry[path]
	if !ok {
		a.registry[path] = &RegistryEntry{
			Path:      path,
			Timestamp: time.Now(),
			Offset:    offset,
		}
	} else {
		if entry.Offset != offset {
			entry.Timestamp = time.Now()
			entry.Offset = offset
		}
	}
}

// recoverRegistry rebuilds the registry from the state file found at path
func (a *Auditor) recoverRegistry(path string) map[string]*RegistryEntry {
	mr, err := ioutil.ReadFile(path)
	if err != nil {
		log.Println(err)
		return make(map[string]*RegistryEntry)
	}
	r, err := a.unmarshalRegistry(mr)
	if err != nil {
		log.Println(err)
		return make(map[string]*RegistryEntry)
	}
	return r
}

// readOnlyRegistryCopy returns a read only copy of the registry
func (a *Auditor) readOnlyRegistryCopy(registry map[string]*RegistryEntry) map[string]RegistryEntry {
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	r := make(map[string]RegistryEntry)
	for path, entry := range registry {
		r[path] = *entry
	}
	return r
}

// flushRegistry writes on disk the registry at the given path
func (a *Auditor) flushRegistry(registry map[string]*RegistryEntry, path string) error {
	r := a.readOnlyRegistryCopy(registry)
	mr, err := a.marshalRegistry(r)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, mr, 0644)
}

// GetLastCommitedOffset returns the last commited offset for a given source
func (a *Auditor) GetLastCommitedOffset(source *config.IntegrationConfigLogSource) (int64, int) {
	r := a.readOnlyRegistryCopy(a.registry)
	entry, ok := r[source.Path]
	if !ok {
		return 0, os.SEEK_END
	}
	return entry.Offset, os.SEEK_CUR
}

// cleanupRegistry removes expired entries from the registry
func (a *Auditor) cleanupRegistry(registry map[string]*RegistryEntry) {
	expireBefore := time.Now().Add(-a.entryTTL)
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	for path, entry := range registry {
		if entry.Timestamp.Before(expireBefore) {
			delete(registry, path)
		}
	}
}

// JsonRegistry represents the registry that will be written on disk
type JsonRegistry struct {
	Version  int
	Registry map[string]RegistryEntry
}

// marshalRegistry marshals a registry
func (a *Auditor) marshalRegistry(registry map[string]RegistryEntry) ([]byte, error) {
	r := JsonRegistry{
		Version:  0,
		Registry: registry,
	}
	return json.Marshal(r)
}

// unmarshalRegistry unmarshals a registry
func (a *Auditor) unmarshalRegistry(b []byte) (map[string]*RegistryEntry, error) {
	var r JsonRegistry
	err := json.Unmarshal(b, &r)
	if err != nil {
		return nil, err
	}
	registry := make(map[string]*RegistryEntry)
	for path, entry := range r.Registry {
		newEntry := entry
		registry[path] = &newEntry
	}
	return registry, nil
}