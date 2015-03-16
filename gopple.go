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
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
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
	buf := new(bytes.Buffer)
	buf2 := new(bytes.Buffer)
	file, err := ioutil.ReadFile(path)
	if err != nil {
		log.Printf("%s -> %s\n", path, err)
		panic(err)
	}
	buf.Write(file)
	buf2.Write(file)
	img, format, err1 := image.Decode(buf)
	im, _, err2 := image.DecodeConfig(buf2)
	if err1 != nil || err2 != nil {
		log.Printf("%s\n -> err1:%s\n -> err2:%s\n", path, err1, err2)
		panic(err1)
	}

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
	var wg sync.WaitGroup
	for _, i := range images {
		i := i
		wg.Add(1)
		go func(i imageInfo) {
			handleImage(i.path, i.info, i.mimetype)
			defer wg.Done()
		}(i)
	}
	wg.Wait()
	fmt.Printf("Done!\n")
}
