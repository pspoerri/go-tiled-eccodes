// grib-tile renders a single WGS84 XYZ tile from a GRIB2 message and writes
// the result as a raw little-endian float32 buffer (W*H values). Useful as a
// smoke test against a real fixture.
//
//	usage: grib-tile FILE Z X Y OUTPUT
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"

	grib "github.com/pspoerri/go-tiled-eccodes"
	"github.com/pspoerri/go-tiled-eccodes/tile"
)

func main() {
	var (
		messageIdx = flag.Int("m", 0, "message index within the file")
		width      = flag.Int("w", 256, "tile width")
		height     = flag.Int("h", 256, "tile height")
		mode       = flag.String("sample", "bicubic", "nearest|bicubic|mode")
	)
	flag.Parse()

	args := flag.Args()
	if len(args) != 5 {
		log.Fatalf("usage: grib-tile [-m INDEX] [-w W] [-h H] [-sample MODE] FILE Z X Y OUTPUT")
	}
	path := args[0]
	z, _ := strconv.Atoi(args[1])
	x, _ := strconv.Atoi(args[2])
	y, _ := strconv.Atoi(args[3])
	outPath := args[4]

	f, err := grib.Open(path)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer f.Close()

	msgs := f.Messages()
	if *messageIdx < 0 || *messageIdx >= len(msgs) {
		log.Fatalf("message index out of range (file has %d messages)", len(msgs))
	}
	m := msgs[*messageIdx]

	smode := tile.Bicubic
	switch *mode {
	case "nearest":
		smode = tile.Nearest
	case "mode":
		smode = tile.Mode
	}

	dst := make([]float32, *width*(*height))
	if err := m.RenderFloat32(grib.TileRequest{
		Tile:   tile.XYZ{Z: z, X: x, Y: y},
		Width:  *width,
		Height: *height,
		Sample: smode,
	}, dst); err != nil {
		log.Fatalf("render: %v", err)
	}

	out, err := os.Create(outPath)
	if err != nil {
		log.Fatalf("create: %v", err)
	}
	defer out.Close()
	if err := binary.Write(out, binary.LittleEndian, dst); err != nil {
		log.Fatalf("write: %v", err)
	}

	// Brief stats summary on stdout.
	var minV, maxV, sum float64
	minV = math.Inf(1)
	maxV = math.Inf(-1)
	var n int
	for _, v := range dst {
		if math.IsNaN(float64(v)) {
			continue
		}
		fv := float64(v)
		if fv < minV {
			minV = fv
		}
		if fv > maxV {
			maxV = fv
		}
		sum += fv
		n++
	}
	if n == 0 {
		fmt.Println("tile is entirely NaN (outside grid extent)")
		return
	}
	fmt.Printf("tile %d/%d/%d  size=%dx%d  valid=%d  min=%.3f  max=%.3f  mean=%.3f\n",
		z, x, y, *width, *height, n, minV, maxV, sum/float64(n))
	fmt.Printf("wrote %d bytes to %s\n", len(dst)*4, outPath)
}
