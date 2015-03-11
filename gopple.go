package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/nfnt/resize"
	"github.com/rakyll/magicmime"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

var db *sql.DB
var thumbdir string

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
	thumb := resize.Resize(32, 0, img, resize.NearestNeighbor)

	thumbhash := createHash(readImageToString(thumb, "png"))

	out, err := os.Create(fmt.Sprintf("%s/%s.png", thumbdir, thumbhash))
	checkErr(err)
	defer out.Close()
	png.Encode(out, thumb)
	return thumbhash
}

func handleImage(path string) (string, string, string, error) {
	file, err := os.Open(path)
	checkErr(err)
	defer file.Close()
	img, format, _ := image.Decode(file)
	im, _, _ := image.DecodeConfig(file)
	x := im.Width
	y := im.Height

	hash := createHash(readImageToString(img, format))
	thumbhash := createThumbnail(img)

	resolution := fmt.Sprintf("%dx%d", x, y)
	return hash, thumbhash, resolution, nil
}

// Gets the path of images within the directory
func walkpath(path string, f os.FileInfo, err error) error {
	mm, err := magicmime.New(magicmime.MAGIC_MIME_TYPE | magicmime.MAGIC_SYMLINK | magicmime.MAGIC_ERROR)
	checkErr(err)

	mi, err := mm.TypeByFile(path)
	if err == nil {
		if mi == "image/jpeg" || mi == "image/png" || mi == "image/gif" {
			hash, thumbhash, res, _ := handleImage(path)
			fmt.Printf("%s -> %s :: %s @ %s\n", path, hash, thumbhash, res)
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
	thumbdir := "./thumbs"
	// Open connection to DB
	db, err := sql.Open("sqlite3", "./gopple.db")
	checkErr(err)

	// Walk directory to get list of images
	root := "./"

	filepath.Walk(root, walkpath)
}
