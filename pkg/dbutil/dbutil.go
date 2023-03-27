package dbutil

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
)

func QuickSetStr(strVal *string, m map[string]interface{}, name string) {
	v, ok := m[name]
	if !ok {
		return
	}
	ival, ok := v.(int64)
	if ok {
		*strVal = strconv.FormatInt(ival, 10)
		return
	}
	str, ok := v.(string)
	if !ok {
		return
	}
	*strVal = str
}

func QuickSetInt64(ival *int64, m map[string]interface{}, name string) {
	v, ok := m[name]
	if !ok {
		return
	}
	sqlInt, ok := v.(int64)
	if !ok {
		return
	}
	*ival = sqlInt
}

func QuickSetBool(bval *bool, m map[string]interface{}, name string) {
	v, ok := m[name]
	if !ok {
		return
	}
	sqlInt, ok := v.(int64)
	if ok {
		if sqlInt > 0 {
			*bval = true
		}
		return
	}
	sqlBool, ok := v.(bool)
	if ok {
		*bval = sqlBool
	}
}

func QuickSetBytes(bval *[]byte, m map[string]interface{}, name string) {
	v, ok := m[name]
	if !ok {
		return
	}
	sqlBytes, ok := v.([]byte)
	if ok {
		*bval = sqlBytes
	}
}

func getByteArr(m map[string]any, name string, def string) ([]byte, bool) {
	v, ok := m[name]
	if !ok {
		return nil, false
	}
	barr, ok := v.([]byte)
	if !ok {
		str, ok := v.(string)
		if !ok {
			return nil, false
		}
		barr = []byte(str)
	}
	if len(barr) == 0 {
		barr = []byte(def)
	}
	return barr, true
}

func QuickSetJson(ptr interface{}, m map[string]interface{}, name string) {
	barr, ok := getByteArr(m, name, "{}")
	if !ok {
		return
	}
	json.Unmarshal(barr, ptr)
}

func QuickSetNullableJson(ptr interface{}, m map[string]interface{}, name string) {
	barr, ok := getByteArr(m, name, "null")
	if !ok {
		return
	}
	json.Unmarshal(barr, ptr)
}

func QuickSetJsonArr(ptr interface{}, m map[string]interface{}, name string) {
	barr, ok := getByteArr(m, name, "[]")
	if !ok {
		return
	}
	json.Unmarshal(barr, ptr)
}

func QuickNullableJson(v interface{}) string {
	if v == nil {
		return "null"
	}
	barr, _ := json.Marshal(v)
	return string(barr)
}

func QuickJson(v interface{}) string {
	if v == nil {
		return "{}"
	}
	barr, _ := json.Marshal(v)
	return string(barr)
}

func QuickJsonBytes(v interface{}) []byte {
	if v == nil {
		return []byte("{}")
	}
	barr, _ := json.Marshal(v)
	return barr
}

func QuickJsonArr(v interface{}) string {
	if v == nil {
		return "[]"
	}
	barr, _ := json.Marshal(v)
	return string(barr)
}

func QuickJsonArrBytes(v interface{}) []byte {
	if v == nil {
		return []byte("[]")
	}
	barr, _ := json.Marshal(v)
	return barr
}

func QuickScanJson(ptr interface{}, val interface{}) error {
	barrVal, ok := val.([]byte)
	if !ok {
		strVal, ok := val.(string)
		if !ok {
			return fmt.Errorf("cannot scan '%T' into '%T'", val, ptr)
		}
		barrVal = []byte(strVal)
	}
	if len(barrVal) == 0 {
		barrVal = []byte("{}")
	}
	return json.Unmarshal(barrVal, ptr)
}

func QuickValueJson(v interface{}) (driver.Value, error) {
	if v == nil {
		return "{}", nil
	}
	barr, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(barr), nil
}
