// Copyright 2024, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package flatjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/wavetermdev/waveterm/pkg/ijson"
)

type FlatJsonVal struct {
	Key string
	Val any
}

type FlatJson []FlatJsonVal

func (fj FlatJson) Pack() ([]byte, error) {
	var buf bytes.Buffer
	for _, fjv := range fj {
		buf.WriteString(fjv.Key)
		buf.WriteString("=")
		barr, err := json.Marshal(fjv.Val)
		if err != nil {
			return nil, fmt.Errorf("error marshalling key %s: %w", fjv.Key, err)
		}
		buf.Write(barr)
		buf.WriteString("\n")
	}
	return buf.Bytes(), nil
}

func Unpack(barr []byte) (FlatJson, error) {
	rtn := make(FlatJson, 0)
	// dont use split, keep a pos and scan for \n
	pos := 0
	lineNum := 1
	for {
		// find newline
		newlinePos := bytes.IndexByte(barr[pos:], '\n')
		if newlinePos == -1 {
			return nil, fmt.Errorf("no newline found, possibly truncated")
		}
		line := barr[pos : pos+newlinePos]
		eqPos := bytes.IndexByte(line, '=')
		if eqPos == -1 {
			return nil, fmt.Errorf("no = found on line %d", lineNum)
		}
		key := string(line[:eqPos])
		valStr := string(line[eqPos+1:])
		var val any
		err := json.Unmarshal([]byte(valStr), &val)
		if err != nil {
			return nil, fmt.Errorf("error unmarshalling value on line %d: %w", lineNum, err)
		}
		rtn = append(rtn, FlatJsonVal{Key: key, Val: val})
		lineNum++
		pos += newlinePos
		if pos == len(barr) {
			break
		}
	}
	return rtn, nil
}

func keyToPath(key string) []any {
	keyParts := strings.Split(key, ":")
	path := make([]any, len(keyParts))
	for idx, keyPart := range keyParts {
		ival, isInt := asInt(keyPart)
		if isInt {
			path[idx] = ival
		} else {
			path[idx] = keyPart
		}
	}
	return path
}

func asInt(s string) (int, bool) {
	ival, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return ival, true
}

func reverseAndRemoteDups(fj FlatJson) FlatJson {
	seen := make(map[string]bool)
	rtn := make(FlatJson, 0)
	// iterate backwards
	for i := len(fj) - 1; i >= 0; i-- {
		if !seen[fj[i].Key] {
			seen[fj[i].Key] = true
			rtn = append(rtn, fj[i])
		}
	}
	return rtn
}

func (fj FlatJson) ToJson(budget int) (map[string]any, map[string]error) {
	rtn := make(map[string]any)
	errs := make(map[string]error)
	fj = reverseAndRemoteDups(fj)
	opts := ijson.SetPathOpts{Budget: budget}
	for _, fjv := range fj {
		if fjv.Key == "" {
			// must be a map
			valMap, ok := fjv.Val.(map[string]any)
			if !ok {
				errs[""] = fmt.Errorf("bad key, does not produce a map")
				continue
			}
			rtn = valMap
			continue
		}
		path := keyToPath(fjv.Key)
		newRtn, err := ijson.SetPath(rtn, path, fjv.Val, &opts)
		if err != nil {
			errs[fjv.Key] = err
		} else {
			newRtnMap, ok := newRtn.(map[string]any)
			if !ok {
				errs[fjv.Key] = fmt.Errorf("bad key, does not produce a map")
				continue
			}
			rtn = newRtnMap
		}
	}
	return rtn, errs
}

func fromJsonArray(v []any, prefix string, fj *FlatJson) error {
	for idx, val := range v {
		newKey := fmt.Sprintf("%s:%d", prefix, idx)
		switch val := val.(type) {
		case map[string]any:
			err := fromJsonMap(val, newKey, fj)
			if err != nil {
				return err
			}
		case []any:
			err := fromJsonArray(val, newKey, fj)
			if err != nil {
				return err
			}
		default:
			*fj = append(*fj, FlatJsonVal{newKey, fmt.Sprintf("%v", val)})
		}
	}
	return nil
}

var validKeyRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

func fromJsonMap(v map[string]any, prefix string, fj *FlatJson) error {
	for key, val := range v {
		if !validKeyRe.MatchString(key) {
			return fmt.Errorf("in map at prefix %q, invalid key %q", prefix, key)
		}
		newKey := key
		if prefix != "" {
			newKey = prefix + ":" + key
		}
		switch val := val.(type) {
		case map[string]any:
			fromJsonMap(val, newKey, fj)
		case []any:
			fromJsonArray(val, newKey, fj)
		default:
			*fj = append(*fj, FlatJsonVal{newKey, fmt.Sprintf("%v", val)})
		}
	}
	return nil
}

func FromJson(v map[string]any) (FlatJson, error) {
	rtn := make(FlatJson, 0)
	err := fromJsonMap(v, "", &rtn)
	if err != nil {
		return nil, err
	}
	return rtn
}
