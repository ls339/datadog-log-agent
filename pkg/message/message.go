// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package message

import (
	"github.com/DataDog/datadog-log-agent/pkg/config"
)

// Message represents a log line sent to datadog, with its metadata
type Message interface {
	Content() []byte
	SetContent([]byte)
	GetOrigin() *MessageOrigin
	SetOrigin(*MessageOrigin)
}

// MessageOrigin represents the Origin of a message
type MessageOrigin struct {
	LogSource *config.IntegrationConfigLogSource
	Offset    int64
}

type message struct {
	content []byte
	Origin  *MessageOrigin
}

// Content returns the content the message, the actual log line
func (m *message) Content() []byte {
	return m.content
}

// SetContent updates the content the message
func (m *message) SetContent(content []byte) {
	m.content = content
}

// GetOrigin returns the Origin from which the message comes
func (m *message) GetOrigin() *MessageOrigin {
	return m.Origin
}

// SetOrigin sets the integration from which the message comes
func (m *message) SetOrigin(Origin *MessageOrigin) {
	m.Origin = Origin
}

// NewMessage returns a new message
func NewMessage(content []byte) *message {
	return &message{
		content: content,
	}
}

// NewOrigin returns a new MessageOrigin
func NewOriginFromOffset(offset int64) *MessageOrigin {
	return &MessageOrigin{
		Offset: offset,
	}
}

// NewNetworkOrigin returns a new MessageOrigin with network only attributes
func NewOriginFromLogSource(Origin *config.IntegrationConfigLogSource) *MessageOrigin {
	return &MessageOrigin{
		LogSource: Origin,
	}
}

// NewFileOrigin returns a new MessageOrigin with file related attributes
func NewOrigin(Origin *config.IntegrationConfigLogSource, offset int64) *MessageOrigin {
	return &MessageOrigin{
		LogSource: Origin,
		Offset:    offset,
	}
}

// StopMessage is used to let a component stop gracefully
type StopMessage struct {
	*message
}

func NewStopMessage() *StopMessage {
	return &StopMessage{
		message: NewMessage(nil),
	}
}

// FileMessage is a message coming from a File
type FileMessage struct {
	*message
}

func NewFileMessage(content []byte) *FileMessage {
	return &FileMessage{
		message: NewMessage(content),
	}
}

// FileMessage is a message coming from a network Source
type NetworkMessage struct {
	*message
}

func NewNetworkMessage(content []byte, source *config.IntegrationConfigLogSource) *NetworkMessage {
	msg := &NetworkMessage{
		message: NewMessage(content),
	}
	msg.SetOrigin(NewOriginFromLogSource(source))
	return msg
}