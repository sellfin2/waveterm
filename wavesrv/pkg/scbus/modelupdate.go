// Copyright 2024, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package scbus

import (
	"encoding/json"
	"reflect"

	"github.com/wavetermdev/waveterm/waveshell/pkg/packet"
)

const ModelUpdateStr = "model"

// A channel for sending model updates to the client
type ModelUpdateChannel[J any] struct {
	ScreenId string
	ClientId string
	ch       chan J
}

func (uch *ModelUpdateChannel[J]) GetChannel() chan J {
	return uch.ch
}

func (uch *ModelUpdateChannel[J]) SetChannel(ch chan J) {
	uch.ch = ch
}

// Match the screenId to the channel
func (sch *ModelUpdateChannel[J]) Match(screenId string) bool {
	if screenId == "" {
		return true
	}
	return screenId == sch.ScreenId
}

// An interface for all model updates
type ModelUpdateItem interface {
	// The key to use when marshalling to JSON and interpreting in the client
	GetType() string
}

// An inner data type for the ModelUpdatePacketType. Stores a collection of model updates to be sent to the client.
type ModelUpdate []ModelUpdateItem

func (mu *ModelUpdate) IsEmpty() bool {
	if mu == nil {
		return true
	}
	muArr := []ModelUpdateItem(*mu)
	return len(muArr) == 0
}

func (mu *ModelUpdate) MarshalJSON() ([]byte, error) {
	rtn := make([]map[string]any, 0)
	for _, u := range *mu {
		m := make(map[string]any)
		m[(u).GetType()] = u
		rtn = append(rtn, m)
	}
	return json.Marshal(rtn)
}

// An UpdatePacket for sending model updates to the client
type ModelUpdatePacketType struct {
	Type string       `json:"type"`
	Data *ModelUpdate `json:"data"`
}

func (*ModelUpdatePacketType) GetType() string {
	return ModelUpdateStr
}

func (upk *ModelUpdatePacketType) IsEmpty() bool {
	if upk == nil {
		return true
	}
	return upk.Data.IsEmpty()
}

// Clean the ClientData in an update, if present
func (upk *ModelUpdatePacketType) Clean() {
	if upk.IsEmpty() {
		return
	}
	for _, item := range *(upk.Data) {
		if i, ok := (item).(CleanableUpdateItem); ok {
			i.Clean()
		}
	}
}

// Add a collection of model updates to the update
func (upk *ModelUpdatePacketType) AddUpdate(items ...ModelUpdateItem) {
	*(upk.Data) = append(*(upk.Data), items...)
}

// Create a new model update packet
func MakeUpdatePacket() *ModelUpdatePacketType {
	return &ModelUpdatePacketType{
		Type: ModelUpdateStr,
		Data: &ModelUpdate{},
	}
}

// Returns the items in the update that are of type I
func GetUpdateItems[I ModelUpdateItem](upk *ModelUpdatePacketType) []*I {
	if upk.IsEmpty() {
		return nil
	}
	ret := make([]*I, 0)
	for _, item := range *(upk.Data) {
		if i, ok := (item).(I); ok {
			ret = append(ret, &i)
		}
	}
	return ret
}

// An interface for model updates that can be cleaned
type CleanableUpdateItem interface {
	Clean()
}

func init() {
	// Register the model update packet type
	packet.RegisterPacketType(ModelUpdateStr, reflect.TypeOf(ModelUpdatePacketType{}))
}
