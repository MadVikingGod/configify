package example

import "io"

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
}

type MyType string
