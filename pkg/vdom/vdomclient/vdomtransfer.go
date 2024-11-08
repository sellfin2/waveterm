// Copyright 2024, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package vdomclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"

	"github.com/wavetermdev/waveterm/pkg/vdom"
)

func transferElemsEqual(t1 *vdom.VDomTransferElem, t2 *vdom.VDomTransferElem) bool {
	if t1 == nil || t2 == nil {
		return false
	}
	if t1.WaveId != t2.WaveId || t1.Tag != t2.Tag || t1.Text != t2.Text {
		return false
	}
	if len(t1.Children) != len(t2.Children) {
		return false
	}
	for i := range t1.Children {
		if t1.Children[i] != t2.Children[i] {
			return false
		}
	}
	return true
}

func (c *Client) ConvertElemsToTransferElems(elems []vdom.VDomElem) []vdom.VDomTransferElem {
	var transferElems []vdom.VDomTransferElem
	var textCacheHits int
	var teCacheHits int
	var numTextNodes int

	// Helper function to recursively process each VDomElem in preorder
	var processElem func(elem vdom.VDomElem) string
	processElem = func(elem vdom.VDomElem) string {
		// Handle #text nodes by generating a unique placeholder ID
		if elem.Tag == "#text" {
			textId := c.TextNodeCache[elem.Text]
			if textId == 0 {
				textId = c.TextNodeNextId
				c.TextNodeNextId++
				c.TextNodeCache[elem.Text] = textId
			} else {
				textCacheHits++
			}
			textIdStr := fmt.Sprintf("text-%d", textId)
			transferElems = append(transferElems, vdom.VDomTransferElem{
				WaveId:   textIdStr,
				Tag:      elem.Tag,
				Text:     elem.Text,
				Props:    nil,
				Children: nil,
			})
			return textIdStr
		}

		// Convert children to WaveId references, handling potential #text nodes
		childrenIds := make([]string, len(elem.Children))
		for i, child := range elem.Children {
			childrenIds[i] = processElem(child) // Children are not roots
		}

		// Create the VDomTransferElem for the current element
		transferElem := vdom.VDomTransferElem{
			WaveId:   elem.WaveId,
			Tag:      elem.Tag,
			Props:    elem.Props,
			Children: childrenIds,
			Text:     elem.Text,
		}
		transferElems = append(transferElems, transferElem)

		return elem.WaveId
	}

	// Start processing each top-level element, marking them as roots
	for _, elem := range elems {
		processElem(elem)
	}

	for _, te := range transferElems {
		if te.Tag == "#text" {
			numTextNodes++
			continue
		}
		if te.WaveId == "" {
			continue
		}
		curTe := c.TransferElemCache[te.WaveId]
		teBytes, _ := json.Marshal(te)
		if bytes.Equal(curTe, teBytes) {
			teCacheHits++
		} else {
			c.TransferElemCache[te.WaveId] = teBytes
		}
	}

	log.Printf("Converted, transferelems: %d/%d, textcache: %d/%d\n", teCacheHits, len(transferElems)-numTextNodes, textCacheHits, numTextNodes)
	return transferElems
}

func (c *Client) DedupTransferElems(elems []vdom.VDomTransferElem) []vdom.VDomTransferElem {
	seen := make(map[string]int) // maps WaveId to its index in the result slice
	var result []vdom.VDomTransferElem

	for _, elem := range elems {
		if idx, exists := seen[elem.WaveId]; exists {
			// Overwrite the previous element with the latest one
			result[idx] = elem
		} else {
			// Add new element and store its index
			seen[elem.WaveId] = len(result)
			result = append(result, elem)
		}
	}

	return result
}

func (c *Client) CreateTransferElems(beUpdate *vdom.VDomBackendUpdate) {
	var vdomElems []vdom.VDomElem
	for idx, reUpdate := range beUpdate.RenderUpdates {
		if reUpdate.VDom == nil {
			continue
		}
		vdomElems = append(vdomElems, *reUpdate.VDom)
		beUpdate.RenderUpdates[idx].VDomWaveId = reUpdate.VDom.WaveId
		beUpdate.RenderUpdates[idx].VDom = nil
	}
	transferElems := c.ConvertElemsToTransferElems(vdomElems)
	transferElems = c.DedupTransferElems(transferElems)
	beUpdate.TransferElems = transferElems
}
