/*
NAME
	sketch - sketch images

SYNOPSIS
	sketch [ -p ] [ -i iter ] [ -l len ] [ -s sec ] file ...

DESCRIPTION
	Sketch uses lines of random length, orientation and color to
	approximate input images. The line colors are randomly selected
	from a palette of all the colors in the input image.

OPTIONS
	-1 num
	      number of first frame (default 1)
	-P    parallelize (slower on short lines)
	-i iter
	      number of iterations (-1 for infinite) (default 5000000)
	-l len
	      line length limit (default 40)
	-n frames
	      number of input frames to sketch
	-p    remove duplicate colors from palette
	-s sec
	      interval between incremental saves, in seconds (default -1)
	-t sec
	      statistics reporting interval, in seconds (default 1)

EXAMPLES
	ffmpeg -i input.webm input%03d.png
	sketch input*.png
	ffmpeg -i frame%03d.png -c:v vp8 output.webm
*/
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"log"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"
)

func bdiff(a, b *image.RGBA, x0, y0, x1, y1 int) float64 {
	dx, dy := x1-x0, y1-y0
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	sx, sy := -1, -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}
	err := dx - dy

	var dif float64
	for {
		dif += calcDiff(a, b, x0, y0)
		if x0 == x1 && y0 == y1 {
			return dif
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func calcDiff(img1, img2 *image.RGBA, x, y int) float64 {
	a := img1.RGBAAt(x, y)
	b := img2.RGBAAt(x, y)
	A := [4]float64{float64(a.R), float64(a.G), float64(a.B), float64(a.A)}
	B := [4]float64{float64(b.R), float64(b.G), float64(b.B), float64(b.A)}
	if true {
		x := (B[0] - A[0]) * (B[0] - A[0])
		x += (B[1] - A[1]) * (B[1] - A[1])
		x += (B[2] - A[2]) * (B[2] - A[2])
		x += (B[3] - A[3]) * (B[3] - A[3])
		return x
	} else {
		// cosine
		x := A[0] * A[0]
		y := B[0] * B[0]
		z := A[0] * B[0]

		x += A[1] * A[1]
		y += B[1] * B[1]
		z += A[1] * B[1]

		x += A[2] * A[2]
		y += B[2] * B[2]
		z += A[2] * B[2]

		x += A[3] * A[3]
		y += B[3] * B[3]
		z += A[3] * B[3]
		return 1 - z/(math.Sqrt(x)*math.Sqrt(y))
	}
}

func bcopy(dst, src *image.RGBA, x0, y0, x1, y1 int) {
	dx, dy := x1-x0, y1-y0
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	sx, sy := -1, -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}
	err := dx - dy

	for {
		dst.SetRGBA(x0, y0, src.RGBAAt(x0, y0))
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func line(img *image.RGBA, x0, y0, x1, y1 int, clr color.RGBA) {
	dx, dy := x1-x0, y1-y0
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	sx, sy := -1, -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}
	err := dx - dy

	for {
		img.SetRGBA(x0, y0, clr)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

var savewait sync.WaitGroup
var saveScratch *image.RGBA

func save(img *image.RGBA, name string) {
	if saveScratch == nil {
		saveScratch = image.NewRGBA(img.Bounds())
	}
	copy(saveScratch.Pix, img.Pix)

	savewait.Wait()
	savewait.Add(1)
	go func() {
		name = fmt.Sprintf("%s.png", name)
		outf, err := os.Create(name)
		if err != nil {
			log.Fatalln(err)
		}
		defer outf.Close()
		png.Encode(outf, img)
		log.Println("wrote", name)
		savewait.Done()
	}()
}

var maxIter int
var frameStart int
var frameLimit int
var lineLen int
var palUniq bool
var saveDelay float64
var statDelay float64
var par bool

func init() {
	flag.IntVar(&maxIter, "i", 5e6, "number of `iter`ations (-1 for infinite)")
	flag.IntVar(&frameStart, "1", 1, "`num`ber of first frame")
	flag.IntVar(&frameLimit, "n", 0, "number of input `frames` to sketch")
	flag.IntVar(&lineLen, "l", 40, "line `len`gth limit")
	flag.BoolVar(&palUniq, "p", false, "remove duplicate colors from palette")
	flag.Float64Var(&saveDelay, "s", -1, "interval between incremental saves, in `sec`onds")
	flag.Float64Var(&statDelay, "t", 1, "statistics reporting interval, in `sec`onds")
	flag.BoolVar(&par, "P", false, "parallelize (slower on short lines)")
}

var incrSaveNum = 1 // when saving incrementally
var saveNum = 1     // when saving finished frames

func sketch(src image.Image) {
	w := src.Bounds().Dx()
	h := src.Bounds().Dy()

	var img *image.RGBA
	switch src.(type) {
	case (*image.RGBA):
		img = src.(*image.RGBA)
	default:
		img = image.NewRGBA(src.Bounds())
		draw.Draw(img, img.Bounds(), src, image.ZP, draw.Src)
	}

	palette := make([]color.RGBA, 0, 1024*1024)
	uniq := make(map[color.RGBA]bool, 50*1024)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if palUniq {
				clr := img.RGBAAt(x, y)
				if _, ok := uniq[clr]; !ok {
					palette = append(palette, clr)
					uniq[clr] = true
				}
			} else {
				palette = append(palette, img.RGBAAt(x, y))
			}
		}
	}
	log.Printf("%d colors in palette\n", len(palette))

	img1 := image.NewRGBA(img.Bounds())
	img2 := image.NewRGBA(img.Bounds())
	bg := color.RGBA{0, 0, 0, 255}
	draw.Draw(img1, img1.Bounds(), &image.Uniform{bg}, image.ZP, draw.Src)
	draw.Draw(img2, img2.Bounds(), &image.Uniform{bg}, image.ZP, draw.Src)

	var lastSaveTime = time.Now()
	var lastStatTime = time.Now()
	var stati int
	var statc int

	for i := 0; i < maxIter || maxIter < 0; i++ {
		stati++
		x1 := rand.Intn(w)
		y1 := rand.Intn(h)
		x2 := -lineLen/2 + x1 + rand.Intn(lineLen)
		y2 := -lineLen/2 + y1 + rand.Intn(lineLen)
		clr := palette[rand.Intn(len(palette))]

		line(img1, x1, y1, x2, y2, clr)

		var diffimg1, diffimg2 float64
		if par {
			var diffwait sync.WaitGroup
			diffwait.Add(2)
			go func() {
				diffimg1 = bdiff(img, img1, x1, y1, x2, y2)
				diffwait.Done()
			}()
			go func() {
				diffimg2 = bdiff(img, img2, x1, y1, x2, y2)
				diffwait.Done()
			}()
			diffwait.Wait()
		} else {
			diffimg1 = bdiff(img, img1, x1, y1, x2, y2)
			diffimg2 = bdiff(img, img2, x1, y1, x2, y2)
		}

		if diffimg1 < diffimg2 {
			// converges
			bcopy(img2, img1, x1, y1, x2, y2)
			statc++
		} else {
			// diverges
			bcopy(img1, img2, x1, y1, x2, y2)
		}
		if i%50 == 0 { // time.Now was bottlenecking
			now := time.Now()
			dur := now.Sub(lastSaveTime)
			if saveDelay > 0 && dur >= time.Duration(saveDelay)*time.Second {
				save(img2, fmt.Sprintf("incr%03d", incrSaveNum))
				incrSaveNum++
				lastSaveTime = time.Now()
			}
			dur = now.Sub(lastStatTime)
			if dur >= time.Duration(statDelay)*time.Second {
				ips := float64(stati) / dur.Seconds()
				cps := float64(statc) / dur.Seconds()
				log.Printf("%8d iters %10.2f iter/s %9.2f converg/s %6.2f%% c/i\n", i, ips, cps, 100*cps/ips)
				stati = 0
				statc = 0
				lastStatTime = time.Now()
			}
		}
	}

	save(img2, fmt.Sprintf("frame%03d", saveNum))
	saveNum++
}

func main() {
	log.SetFlags(0)
	rand.Seed(1)
	flag.Parse()

	frame := frameStart

	if flag.NArg() > 0 {
		for i := 0; i < flag.NArg(); i++ {
			log.Println("opening", flag.Arg(i))
			f, err := os.Open(flag.Arg(i))
			if err != nil {
				break
			}
			src, _, err := image.Decode(f)
			if err != nil {
				f.Close()
				log.Fatal(err)
			}
			f.Close()

			sketch(src)
			frame++
		}
		log.Println("end of frames")
		return
	}

	for {
		if frameLimit > 1 && frame-frameStart > frameLimit {
			break
		}
		name := fmt.Sprintf("input%03d.png", frame)
		log.Printf("opening %s", name)
		f, err := os.Open(name)
		if err != nil {
			log.Fatal(err)
		}
		src, _, err := image.Decode(f)
		if err != nil {
			f.Close()
			log.Fatal(err)
		}
		f.Close()

		sketch(src)
		frame++
	}
	log.Print("end of frames")
}
