// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package sstore

import (
	"github.com/wavetermdev/waveterm/wavesrv/pkg/dbutil"
)

var quickSetStr = dbutil.QuickSetStr
var quickSetInt64 = dbutil.QuickSetInt64
var quickSetInt = dbutil.QuickSetInt
var quickSetBool = dbutil.QuickSetBool
var quickSetBytes = dbutil.QuickSetBytes
var quickSetJson = dbutil.QuickSetJson
var quickSetNullableJson = dbutil.QuickSetNullableJson
var quickSetJsonArr = dbutil.QuickSetJsonArr
var quickNullableJson = dbutil.QuickNullableJson
var quickJson = dbutil.QuickJson
var quickJsonArr = dbutil.QuickJsonArr
var quickScanJson = dbutil.QuickScanJson
var quickValueJson = dbutil.QuickValueJson
