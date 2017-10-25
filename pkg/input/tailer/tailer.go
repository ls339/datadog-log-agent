// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package tailer

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/decoder"
	"github.com/DataDog/datadog-log-agent/pkg/message"
)

const defaultSleepDuration = 1 * time.Second
const defaultCloseTimeout = 60 * time.Second

// Tailer tails one file and sends messages to an output channel
type Tailer struct {
	path string
	file *os.File

	lastOffset        int64
	shouldTrackOffset bool

	outputChan chan message.Message
	d          *decoder.Decoder
	source     *config.IntegrationConfigLogSource

	sleepDuration time.Duration
	sleepMutex    sync.Mutex

	closeTimeout time.Duration
	shouldStop   bool
	stopTimer    *time.Timer
	stopMutex    sync.Mutex
}

// NewTailer returns an initialized Tailer
func NewTailer(outputChan chan message.Message, source *config.IntegrationConfigLogSource) *Tailer {
	return &Tailer{
		path:       source.Path,
		outputChan: outputChan,
		d:          decoder.InitializedDecoder(),
		source:     source,

		lastOffset:        0,
		shouldTrackOffset: true,

		sleepDuration: defaultSleepDuration,
		sleepMutex:    sync.Mutex{},
		shouldStop:    false,
		stopMutex:     sync.Mutex{},
		closeTimeout:  defaultCloseTimeout,
	}
}

// Stop lets  the tailer stop
func (t *Tailer) Stop(shouldTrackOffset bool) {
	t.stopMutex.Lock()
	t.shouldStop = true
	t.shouldTrackOffset = shouldTrackOffset
	t.stopTimer = time.NewTimer(t.closeTimeout)
	t.stopMutex.Unlock()
}

// onStop handles the housekeeping when we stop the tailer
func (t *Tailer) onStop() {
	t.stopMutex.Lock()
	t.d.Stop()
	log.Println("Closing", t.path)
	t.file.Close()
	t.stopTimer.Stop()
	t.stopMutex.Unlock()
}

// tailFrom let's the tailer open a file and tail from whence
func (t *Tailer) tailFrom(offset int64, whence int) error {
	t.d.Start()
	go t.forwardMessages()
	return t.startReading(offset, whence)
}

func (t *Tailer) startReading(offset int64, whence int) error {
	fullpath, err := filepath.Abs(t.path)
	if err != nil {
		return err
	}
	log.Println("Opening", t.path)
	f, err := os.Open(fullpath)
	if err != nil {
		return err
	}
	ret, _ := f.Seek(offset, whence)
	t.file = f
	t.lastOffset = ret

	go t.readForever()
	return nil
}

// tailFromBegining lets the tailer start tailing its file
// from the begining
func (t *Tailer) tailFromBegining() error {
	return t.tailFrom(0, os.SEEK_SET)
}

// tailFromBegining lets the tailer start tailing its file
// from the end
func (t *Tailer) tailFromEnd() error {
	return t.tailFrom(0, os.SEEK_END)
}

// reset makes the tailer seek the begining of its file
func (t *Tailer) reset() {
	t.file.Seek(0, os.SEEK_SET)
	t.setLastOffset(0)
}

// forwardMessages lets the Tailer forward log messages to the output channel
func (t *Tailer) forwardMessages() {
	for msg := range t.d.OutputChan {

		_, ok := msg.(*message.StopMessage)
		if ok {
			return
		}

		fileMsg := message.NewFileMessage(msg.Content())
		msgOffset := msg.GetOrigin().Offset
		if !t.shouldTrackOffset {
			msgOffset = 0
		}
		msgOrigin := message.NewOrigin(t.source, msgOffset)
		fileMsg.SetOrigin(msgOrigin)
		t.outputChan <- fileMsg
	}
}

// readForever lets the tailer tail the content of a file
// until it is closed.
func (t *Tailer) readForever() {
	for {
		if t.shouldHardStop() {
			t.onStop()
			return
		}

		inBuf := make([]byte, 4096)
		n, err := t.file.Read(inBuf)
		if err == io.EOF {
			if t.shouldSoftStop() {
				t.onStop()
				return
			}
			t.wait()
			continue
		}
		if err != nil {
			log.Println("Err:", err)
			return
		}
		if n == 0 {
			t.wait()
			continue
		}
		t.d.InputChan <- decoder.NewPayload(inBuf[:n], t.GetLastOffset())
		t.incrementLastOffset(n)
	}
}

func (t *Tailer) shouldHardStop() bool {
	t.stopMutex.Lock()
	defer t.stopMutex.Unlock()
	if t.stopTimer != nil {
		select {
		case <-t.stopTimer.C:
			return true
		default:
		}
	}
	return false
}

func (t *Tailer) shouldSoftStop() bool {
	t.stopMutex.Lock()
	defer t.stopMutex.Unlock()
	return t.shouldStop
}

func (t *Tailer) incrementLastOffset(n int) {
	atomic.AddInt64(&t.lastOffset, int64(n))
}

func (t *Tailer) setLastOffset(n int64) {
	atomic.StoreInt64(&t.lastOffset, n)
}

func (t *Tailer) GetLastOffset() int64 {
	return atomic.LoadInt64(&t.lastOffset)
}

// wait lets the tailer sleep for a bit
func (t *Tailer) wait() {
	t.sleepMutex.Lock()
	defer t.sleepMutex.Unlock()
	time.Sleep(t.sleepDuration)
}