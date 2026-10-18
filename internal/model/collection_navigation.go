package model

type Column uint8

const (
	ColumnList Column = iota
	ColumnCollections
	ColumnDetail
)
