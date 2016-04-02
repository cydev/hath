package storage

import (
	"encoding/binary"
	"os"
)

// An IndexBackend describes a backend that is used for index store.
type IndexBackend interface {
	ReadAt(b []byte, off int64) (int, error)
	WriteAt(b []byte, off int64) (int, error)
	Stat() (os.FileInfo, error)
}

// Index uses IndexBackend to store and retrieve Links
type Index struct {
	Backend IndexBackend
}

// ReadBuff returns Link with provided id, using b as buffer
func (i Index) ReadBuff(id int64, b []byte) (Link, error) {
	l := Link{}
	n, err := i.Backend.ReadAt(b, getLinkOffset(id))
	if err != nil {
		return l, err
	}
	l.Read(b[:n])
	return l, nil
}

// getLinkOffset returns offset in index for link with provided file id.
// Link.ID starts from 0, so getLinkOffset(0) == 0, getLinkOffset(1) == LinkStructureSize.
func getLinkOffset(id int64) int64 {
	return id * LinkStructureSize
}

// Link is index entry that links file id to offset
type Link struct {
	ID     int64 // ID of file
	Offset int64 // Offset for file in bulk
}

// LinkStructureSize is minimum buf length required in Link.{Read,Put} and is 128 bit or 16 byte.
const LinkStructureSize = 8 * 2

// Put link to byte slice using binary.Put(U)Variant for all fields, returns write size in bytes.
func (l Link) Put(b []byte) int {
	var offset int
	offset += binary.PutVarint(b[offset:], l.ID)
	offset += binary.PutVarint(b[offset:], l.Offset)
	return offset
}

// Read file from byte slice using binary.Put(U)Variant for all fields, returns read size in bytes.
func (l *Link) Read(b []byte) int {
	var offset, read int
	l.ID, read = binary.Varint(b[offset:])
	offset += read
	l.Offset, read = binary.Varint(b[offset:])
	return offset + read
}
