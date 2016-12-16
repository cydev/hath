package hath

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dineshappavoo/basex"
)

const (
	keyStampEnd  = "hotlinkthis"
	prefixLenght = 2
	// HashSize is length of sha1 hash in bytes
	HashSize             = 20
	sizeBytes            = 4
	resolutionBytes      = 2
	fileBytes            = 38
	keyStampLength       = 10
	staticRangeBytes     = 2
	staticRangeHexLength = 4
	staticRangeDelimiter = ";"

	// file size limitations
	size10MB = 1024 * 1024 * 10
	// FileMaximumSize is maximum image size in hath
	FileMaximumSize = size10MB
)

// FileType represents file format of image
type FileType byte

// StaticRange is prefix for static ranges assigned to user
type StaticRange [staticRangeBytes]byte

func (s StaticRange) String() string {
	return hex.EncodeToString(s[:])
}

// ParseStaticRange parses hex string static range start
func ParseStaticRange(s string) (r StaticRange, err error) {
	if len(s) != staticRangeHexLength {
		return r, io.ErrUnexpectedEOF
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return r, err
	}
	copy(r[:], b[:])
	return r, err
}

// StaticRanges contain ranges
type StaticRanges map[StaticRange]bool

// Contains returns true if file f is in static ranges
func (s StaticRanges) Contains(f File) bool {
	return s[f.Range()]
}

// Add static range
func (s StaticRanges) Add(r StaticRange) {
	s[r] = true
}

// Remove static range
func (s StaticRanges) Remove(r StaticRange) {
	delete(s, r)
}

// Count of static ranges
func (s StaticRanges) Count() int {
	return len(s)
}

func (s StaticRanges) String() string {
	var elems []string
	for k := range s {
		elems = append(elems, k.String())
	}
	sort.Strings(elems)
	return strings.Join(elems, staticRangeDelimiter)
}

func (f FileType) String() string {
	if f == JPG {
		return "jpg"
	}
	if f == PNG {
		return "png"
	}
	if f == GIF {
		return "gif"
	}
	return "tmp"
}

const (
	// JPG image
	JPG FileType = iota
	// PNG image
	PNG
	// GIF animation
	GIF
	// UnknownImage is not supported format
	UnknownImage
)

var (
	// ContentTypes is map of file types to content types
	ContentTypes = map[FileType]string{
		JPG:          "image/jpeg",
		PNG:          "image/png",
		GIF:          "image/gif",
		UnknownImage: "application/octet-stream",
	}
)

var (
	// ErrFileTypeUnknown when FileType is UnknownImage
	ErrFileTypeUnknown = errors.New("hath => file type unknown")
	// ErrHashBadLength when hash size is not HashSize
	ErrHashBadLength = errors.New("hath => hash of image has bad length")
)

// ParseFileType returns FileType from string
func ParseFileType(t string) FileType {
	switch strings.ToLower(t) {
	case "jpg", "jpeg":
		return JPG
	case "png":
		return PNG
	case "gif":
		return GIF
	default:
		return UnknownImage
	}
}

// File is hath file representation
// total 20 + 4 + 2 + 2 + 1 + 8 + 1 = 38 bytes
// in memory = 56 bytes
type File struct {
	Hash [HashSize]byte `json:"hash"` // 20 byte
	Type FileType       `json:"type"` // 1 byte
	// Static files should never be removed
	Static bool  `json:"static"` // 1 byte
	Size   int64 `json:"size"`   // 4 byte (maximum size 4095mb)
	Width  int   `json:"width"`  // 2 byte
	Height int   `json:"height"` // 2 byte
	// LastUsage is Unix timestamp
	LastUsage int64 `json:"last_usage"` // 8 byte (can be optimized)
}

// ContentType of image
func (f File) ContentType() string {
	switch f.Type {
	case JPG:
		return "image/jpeg"
	case PNG:
		return "image/png"
	case GIF:
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}

// Range returns static range of file
func (f File) Range() (r StaticRange) {
	copy(r[:], f.Hash[:staticRangeBytes])
	return r
}

// InRange returns true if file is in static range r
func (f File) InRange(r StaticRange) bool {
	return bytes.Equal(r[:], f.Hash[:staticRangeBytes])
}

// Bytes serializes file info into byte array
func (f File) Bytes() []byte {
	var result [fileBytes]byte
	var buff [8]byte
	cursor := 0

	// writing hash
	copy(result[cursor:HashSize], f.Hash[:])
	cursor += HashSize

	// writing type
	result[cursor] = byte(f.Type)
	cursor++

	// writing static
	if f.Static {
		result[cursor] = 255
	}
	cursor++

	// Size is 64bit, or 8 byte
	// little endian is 1111111111000000000
	// we want only first right 4 byte
	binary.LittleEndian.PutUint64(buff[:], uint64(f.Size))
	copy(result[cursor:cursor+sizeBytes], buff[:sizeBytes])
	cursor += sizeBytes

	// writing height
	binary.LittleEndian.PutUint64(buff[:], uint64(f.Height))
	copy(result[cursor:cursor+resolutionBytes], buff[:resolutionBytes])
	cursor += resolutionBytes

	// writing width
	binary.LittleEndian.PutUint64(buff[:], uint64(f.Width))
	copy(result[cursor:cursor+resolutionBytes], buff[:resolutionBytes])
	cursor += resolutionBytes

	// writing time
	binary.LittleEndian.PutUint64(buff[:], uint64(f.LastUsage))
	copy(result[cursor:cursor+8], buff[:])
	cursor += 8
	return result[:]
}

// FileFromBytes deserializes byte slice into file
func FileFromBytes(result []byte) (f File, err error) {
	return f, FileFromBytesTo(result, &f)
}

// FileFromBytesTo deserializes byte slice into file by pointer
func FileFromBytesTo(result []byte, f *File) error {
	if len(result) != fileBytes {
		return ErrFileInconsistent
	}
	var buff [8]byte
	cursor := 0
	// reading hash
	copy(f.Hash[:], result[cursor:HashSize])
	cursor += HashSize

	// reading type
	f.Type = FileType(result[cursor])
	cursor++

	// reading static
	f.Static = result[cursor] == 255
	cursor++

	// Size is 64bit, or 8 byte
	// little endian is 1111111111000000000
	// we want only first right 4 byte
	copy(buff[:sizeBytes], result[cursor:cursor+sizeBytes])
	f.Size = int64(binary.LittleEndian.Uint64(buff[:]))
	cursor += sizeBytes

	// reading height
	buff = [8]byte{} // buffer reset
	copy(buff[:resolutionBytes], result[cursor:cursor+resolutionBytes])
	f.Height = int(binary.LittleEndian.Uint64(buff[:]))
	cursor += resolutionBytes

	// reading width
	buff = [8]byte{} // buffer reset
	copy(buff[:resolutionBytes], result[cursor:cursor+resolutionBytes])
	f.Width = int(binary.LittleEndian.Uint64(buff[:]))
	cursor += resolutionBytes

	// reading time
	buff = [8]byte{} // buffer reset
	copy(buff[:], result[cursor:cursor+8])
	f.LastUsage = int64(binary.LittleEndian.Uint64(buff[:]))

	return nil
}

func (f File) indexKey() []byte {
	timeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timeBytes, uint64(f.LastUsage))
	elems := [][]byte{
		timeBytes,
		f.Hash[:],
	}
	return bytes.Join(elems, nil)
}

// LastUsageBefore returns true, if last usage occured before deadline t
func (f File) LastUsageBefore(t time.Time) bool {
	return t.Unix() < f.LastUsage
}

// Dir is first prefixLenght chars of file hash
func (f File) Dir() string {
	return f.HexID()[:prefixLenght]
}

// Path returns relative path to file
func (f File) Path() string {
	return path.Join(f.Dir(), f.String())
}

// Use sets LastUsage to current time
func (f *File) Use() {
	f.LastUsage = time.Now().Unix()
}

// HexID returns hex representation of hash
func (f File) HexID() string {
	return fmt.Sprintf("%x", f.Hash)
}

// SetHash sets hash from string
func (f *File) SetHash(s string) error {
	hash, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	if len(hash) != HashSize {
		return ErrHashBadLength
	}
	copy(f.Hash[:], hash[:HashSize])
	return nil
}

// Buffer creates buffer with size of file
func (f *File) Buffer() *bytes.Buffer {
	return bytes.NewBuffer(make([]byte, 0, f.Size))
}

// FileFromID generates new File from provided ID
func FileFromID(fileid string) (f File, err error) {
	elems := strings.Split(fileid, keyStampDelimiter)
	if len(elems) != 5 {
		return f, io.ErrUnexpectedEOF
	}
	if err = f.SetHash(elems[0]); err != nil {
		return
	}
	f.Size, err = strconv.ParseInt(elems[1], 10, 64)
	if err != nil {
		return
	}
	f.Width, err = strconv.Atoi(elems[2])
	if err != nil {
		return
	}
	f.Height, err = strconv.Atoi(elems[3])
	if err != nil {
		return
	}
	f.Type = ParseFileType(elems[4])
	f.LastUsage = time.Now().Unix()
	return f, err
}

func (f File) String() string {
	elems := []string{
		f.HexID(),
		sInt64(f.Size),
		strconv.Itoa(f.Width),
		strconv.Itoa(f.Height),
		f.Type.String(),
	}
	return strings.Join(elems, keyStampDelimiter)
}

// KeyStamp generates file key for provided timestamp
func (f File) KeyStamp(key string, timestamp int64) string {
	elems := []string{
		sInt64(timestamp),
		f.String(),
		key,
		keyStampEnd,
	}
	toHash := strings.Join(elems, keyStampDelimiter)
	hash := sha1.Sum([]byte(toHash))
	return fmt.Sprintf("%x", hash)[:keyStampLength]
}

// Basex returns basex representation of hash
func (f File) Basex() string {
	d := f.ByteID()
	n := big.NewInt(0)
	n.SetBytes(d)
	s, _ := basex.Encode(n.String())
	return s
}

// Marshal serializes file info
func (f File) Marshal() ([]byte, error) {
	return f.Bytes(), nil
}

// UnmarshalFile deserializes file info fron byte array
func UnmarshalFile(data []byte) (f File, err error) {
	return f, FileFromBytesTo(data, &f)
}

// UnmarshalFileTo deserializes file info fron byte array by pointer
func UnmarshalFileTo(data []byte, f *File) error {
	return FileFromBytesTo(data, f)
}

// ByteID returns []byte for file hash
func (f File) ByteID() []byte {
	return f.Hash[:]
}
