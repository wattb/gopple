package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/nfnt/resize"
	"github.com/rakyll/magicmime"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
)

var db *sql.DB
var thumbdir string
var images []imageInfo

type imageInfo struct {
	path     string
	info     os.FileInfo
	mimetype string
}

// Adds an image to the database
// Creates a hash of the image, a thumbnail, and a hash of that
func addImage(path string, f os.FileInfo, mimetype string, hash string, thumbhash string, resolution string) {
	stmt, err := db.Prepare("INSERT INTO images(path, size, hash, thumbhash, resolution, mimetype) values(?,?,?,?,?,?)")
	checkErr(err)
	stmt.Exec(path, f.Size, hash, thumbhash, resolution, mimetype)
	log.Printf("Added image %s to DB.\n", path)
}

// Hashes the file and returns it as a string
func createHash(buf string) string {
	h := sha256.New()
	fmt.Fprintf(h, buf)
	hash := fmt.Sprintf("%x", h.Sum(nil))
	return hash
}

func readImageToString(img image.Image, format string) string {
	buf := new(bytes.Buffer)
	if format == "png" {
		png.Encode(buf, img)
	} else if format == "jpeg" {
		jpeg.Encode(buf, img, nil)
	} else if format == "gif" {
		gif.Encode(buf, img, nil)
	}
	return string(buf.Bytes())
}

// Creates a thumbnail and writes it to file, returning the thumbnail hash
func createThumbnail(img image.Image) string {
	thumb := resize.Resize(16, 16, img, resize.NearestNeighbor)

	thumbhash := createHash(readImageToString(thumb, "png"))

	out, err := os.Create(fmt.Sprintf("%s/%s.png", thumbdir, thumbhash))
	checkErr(err)
	png.Encode(out, thumb)
	out.Close()
	return thumbhash
}

func handleImage(path string, f os.FileInfo, mimetype string) {
	file, err := os.Open(path)
	if err != nil {
		lo := fmt.Sprintf("%s -> %s", path, err)
		log.Fatal(lo)
		file.Close()
		panic(err)
	}
	img, format, err1 := Decode(file)
	im, _, err2 := DecodeConfig(file)
	if err1 != nil || err2 != nil {
		lo := fmt.Sprintf("%s -> %s, %s", path, err1, err2)
		log.Fatal(lo)
	}
	defer file.Close()

	x := im.Width
	y := im.Height

	hash := createHash(readImageToString(img, format))
	thumbhash := createThumbnail(img)

	resolution := fmt.Sprintf("%dx%d", x, y)

	//addImage(path, f, mimetype, hash, thumbhash, resolution)

	l := fmt.Sprintf("%s -> %s :: %s @ %s\n", path, hash, thumbhash, resolution)
	log.Printf(l)
}

// Gets the path of images within the directory
func walkpath(path string, f os.FileInfo, err error) error {
	mm, err := magicmime.New(magicmime.MAGIC_MIME_TYPE | magicmime.MAGIC_SYMLINK | magicmime.MAGIC_ERROR)
	if err != nil {
		lo := fmt.Sprintf("%s -> %s", path, err)
		log.Fatal(lo)
		panic(err)
	}

	mi, err := mm.TypeByFile(path)
	if err == nil {
		if mi == "image/jpeg" || mi == "image/png" || mi == "image/gif" {
			images = append(images, imageInfo{path, f, mi})
		}
	}
	return nil
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
		panic(err)
	}
}

func main() {
	thumbdir = ".thumbs"
	// Open connection to DB
	db, _ = sql.Open("sqlite3", "./gopple.db")

	// Walk directory to get list of images
	root := "./"

	filepath.Walk(root, walkpath)
	for n, i := range images {
		fmt.Printf("%d:: %s %s\n", n, i.path, i.mimetype)
		go handleImage(i.path, i.info, i.mimetype)
	}
}

// ErrFormat indicates that decoding encountered an unknown format.
var ErrFormat = errors.New("image: unknown format")

// A format holds an image format's name, magic header and how to decode it.
type format struct {
	name, magic  string
	decode       func(io.Reader) (image.Image, error)
	decodeConfig func(io.Reader) (image.Config, error)
}

// Formats is the list of registered formats.
var formats []format

// RegisterFormat registers an image format for use by Decode.
// Name is the name of the format, like "jpeg" or "png".
// Magic is the magic prefix that identifies the format's encoding. The magic
// string can contain "?" wildcards that each match any one byte.
// Decode is the function that decodes the encoded image.
// DecodeConfig is the function that decodes just its configuration.
func RegisterFormat(name, magic string, decode func(io.Reader) (image.Image, error), decodeConfig func(io.Reader) (image.Config, error)) {
	formats = append(formats, format{name, magic, decode, decodeConfig})
}

// A reader is an io.Reader that can also peek ahead.
type reader interface {
	io.Reader
	Peek(int) ([]byte, error)
}

// asReader converts an io.Reader to a reader.
func asReader(r io.Reader) reader {
	if rr, ok := r.(reader); ok {
		return rr
	}
	return bufio.NewReader(r)
}

// Match reports whether magic matches b. Magic may contain "?" wildcards.
func match(magic string, b []byte) bool {
	if len(magic) != len(b) {
		return false
	}
	for i, c := range b {
		if magic[i] != c && magic[i] != '?' {
			return false
		}
	}
	return true
}

// Sniff determines the format of r's data.
func sniff(r reader) (format, error) {
	for _, f := range formats {
		b, err := r.Peek(len(f.magic))
		if err == nil && match(f.magic, b) {
			return f, nil
		} else {
			return format{}, err
		}
	}
	return format{}, nil
}

// Decode decodes an image that has been encoded in a registered format.
// The string returned is the format name used during format registration.
// Format registration is typically done by an init function in the codec-
// specific package.
func Decode(r io.Reader) (image.Image, string, error) {
	rr := asReader(r)
	f, err := sniff(rr)
	if err != nil || f.decode == nil {
		return nil, "", err
	}
	m, _ := f.decode(rr)
	return m, f.name, err
}

// DecodeConfig decodes the color model and dimensions of an image that has
// been encoded in a registered format. The string returned is the format name
// used during format registration. Format registration is typically done by
// an init function in the codec-specific package.
func DecodeConfig(r io.Reader) (image.Config, string, error) {
	rr := asReader(r)
	f, err := sniff(rr)
	if err != nil || f.decodeConfig == nil {
		return image.Config{}, "", err
	}
	c, err := f.decodeConfig(rr)
	return c, f.name, err
}
