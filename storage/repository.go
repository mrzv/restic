package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"

	"github.com/fd0/khepri/hashing"
)

const (
	dirMode    = 0700
	objectPath = "objects"
	refPath    = "refs"
	tempPath   = "tmp"
)

type Repository interface {
	Put(reader io.Reader) (ID, error)
	PutFile(path string) (ID, error)
	Get(ID) (io.Reader, error)
	Test(ID) (bool, error)
	Remove(ID) error
	Link(name string, id ID) error
	Unlink(name string) error
	Resolve(name string) (ID, error)
}

var (
	ErrIDDoesNotExist = errors.New("ID does not exist")
)

// References content within a repository.
type ID []byte

func (id ID) String() string {
	return hex.EncodeToString(id)
}

// Equal compares an ID to another other.
func (id ID) Equal(other ID) bool {
	return bytes.Equal(id, other)
}

// EqualString compares this ID to another one, given as a string.
func (id ID) EqualString(other string) (bool, error) {
	s, err := hex.DecodeString(other)
	if err != nil {
		return false, err
	}

	return id.Equal(ID(s)), nil
}

// Name stands for the alias given to an ID.
type Name string

func (n Name) Encode() string {
	return url.QueryEscape(string(n))
}

type Dir struct {
	path string
	hash func() hash.Hash
}

// NewDir creates a new dir-baked repository at the given path.
func NewDir(path string) (*Dir, error) {
	d := &Dir{
		path: path,
		hash: sha256.New,
	}

	err := d.create()

	if err != nil {
		return nil, err
	}

	return d, nil
}

func (r *Dir) create() error {
	dirs := []string{
		r.path,
		path.Join(r.path, objectPath),
		path.Join(r.path, refPath),
		path.Join(r.path, tempPath),
	}

	for _, dir := range dirs {
		err := os.MkdirAll(dir, dirMode)
		if err != nil {
			return err
		}
	}

	return nil
}

// SetHash changes the hash function used for deriving IDs. Default is SHA256.
func (r *Dir) SetHash(h func() hash.Hash) {
	r.hash = h
}

// Path returns the directory used for this repository.
func (r *Dir) Path() string {
	return r.path
}

// Put saves content and returns the ID.
func (r *Dir) Put(reader io.Reader) (ID, error) {
	// save contents to tempfile, hash while writing
	file, err := ioutil.TempFile(path.Join(r.path, tempPath), "temp-")
	if err != nil {
		return nil, err
	}

	rd := hashing.NewReader(reader, r.hash)
	_, err = io.Copy(file, rd)
	if err != nil {
		return nil, err
	}

	err = file.Close()
	if err != nil {
		return nil, err
	}

	// move file to final name using hash of contents
	id := ID(rd.Hash())
	filename := path.Join(r.path, objectPath, id.String())
	err = os.Rename(file.Name(), filename)
	if err != nil {
		return nil, err
	}

	return id, nil
}

// PutFile saves a file's content to the repository and returns the ID.
func (r *Dir) PutFile(path string) (ID, error) {
	f, err := os.Open(path)
	defer f.Close()
	if err != nil {
		return nil, err
	}

	return r.Put(f)
}

// Test returns true if the given ID exists in the repository.
func (r *Dir) Test(id ID) (bool, error) {
	// try to open file
	file, err := os.Open(path.Join(r.path, objectPath, id.String()))
	defer func() {
		file.Close()
	}()

	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// Get returns a reader for the content stored under the given ID.
func (r *Dir) Get(id ID) (io.Reader, error) {
	// try to open file
	file, err := os.Open(path.Join(r.path, objectPath, id.String()))
	if err != nil {
		return nil, err
	}

	return file, nil
}

// Remove removes the content stored at ID.
func (r *Dir) Remove(id ID) error {
	return os.Remove(path.Join(r.path, objectPath, id.String()))
}

// Unlink removes a named ID.
func (r *Dir) Unlink(name string) error {
	return os.Remove(path.Join(r.path, refPath, Name(name).Encode()))
}

// Link assigns a name to an ID. Name must be unique in this repository and ID must exist.
func (r *Dir) Link(name string, id ID) error {
	exist, err := r.Test(id)
	if err != nil {
		return err
	}

	if !exist {
		return ErrIDDoesNotExist
	}

	// create file, write id
	f, err := os.Create(path.Join(r.path, refPath, Name(name).Encode()))
	defer f.Close()

	if err != nil {
		return err
	}

	f.Write([]byte(hex.EncodeToString(id)))
	return nil
}

// Resolve returns the ID associated with the given name.
func (r *Dir) Resolve(name string) (ID, error) {
	f, err := os.Open(path.Join(r.path, refPath, Name(name).Encode()))
	defer f.Close()
	if err != nil {
		return nil, err
	}

	// read hex string
	l := r.hash().Size()
	buf := make([]byte, l*2)
	_, err = io.ReadFull(f, buf)

	if err != nil {
		return nil, err
	}

	id := make([]byte, l)
	_, err = hex.Decode(id, buf)
	if err != nil {
		return nil, err
	}

	return ID(id), nil
}