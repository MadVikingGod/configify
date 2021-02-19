package example

import (
	"bytes"
	"io"

	s "sync"

	"github.com/pkg/errors"
)

type config struct {
	myType MyType
	color  string
	Height int
	array  [3]int
	slice  []int
	maps   map[string]string
	funcs  func(int) error
	point  *string
	inter  io.Reader
	MyType
	buf   bytes.Buffer
	buf2  *s.WaitGroup
	frame errors.Frame
}

type MyType string
