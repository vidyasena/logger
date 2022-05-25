package logger

import (
	"reflect"
	"time"
)

const (
	File = "file"
)

const (
	LogTypeTDR = "TDR"
	LogTypeSYS = "SYS"
)

const separator = "|"

var (
	TypeSliceOfBytes = reflect.TypeOf([]byte(nil))
	TypeTime         = reflect.TypeOf(time.Time{})
)
