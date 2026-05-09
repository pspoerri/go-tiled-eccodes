// grib-inspect: minimal viewer that lists messages, headers, and grid summary
// for one or more GRIB2 files. Used as a smoke test against real fixtures.
package main

import (
	"fmt"
	"log"
	"os"

	grib "github.com/pspoerri/go-tiled-eccodes"
	gridpkg "github.com/pspoerri/go-tiled-eccodes/grid"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: grib-inspect FILE [FILE...]")
	}
	for _, path := range os.Args[1:] {
		f, err := grib.Open(path)
		if err != nil {
			log.Fatalf("open %s: %v", path, err)
		}
		fmt.Printf("== %s ==\n", path)
		for i, m := range f.Messages() {
			h := m.Header()
			fmt.Printf("[%3d] disc=%d cat=%d num=%d ref=%s fcst=%d gridT=%d packT=%d N=%d (%dx%d)\n",
				i, h.Discipline, h.ParameterCategory, h.ParameterNumber,
				h.ReferenceTime.Format("2006-01-02T15:04Z"),
				h.ForecastTime,
				h.GridTemplate, h.DataTemplate, h.NumDataPoints, h.Ni, h.Nj,
			)
			if g, err := m.Grid(); err == nil {
				if ll, ok := g.(gridpkg.LatLon); ok {
					fmt.Printf("       lat[%.4f .. %.4f] lon[%.4f .. %.4f] di=%g dj=%g\n",
						ll.La1, ll.La2, ll.Lo1, ll.Lo2, ll.Di, ll.Dj)
				}
			}
		}
		f.Close()
	}
}
