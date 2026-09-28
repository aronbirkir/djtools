// Command genicon renders the djtools app icon to icon.png: a vinyl record on
// the same violet macOS-style rounded square as MP3 Renamer's icon.
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

const size = 1024

var (
	white  = [3]float64{255, 255, 255}
	disc   = [3]float64{0x1b, 0x1e, 0x25}
	groove = [3]float64{0x2c, 0x30, 0x3b}
)

func main() {
	out := "icon.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4 // supersampling per axis
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+0.5)/ss
					py := float64(y) + (float64(sy)+0.5)/ss
					if c, ok := sample(px, py); ok {
						r, g, b, a = r+c[0], g+c[1], b+c[2], a+1
					}
				}
			}
			if a == 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r / a), G: uint8(g / a), B: uint8(b / a),
				A: uint8(a / (ss * ss) * 255),
			})
		}
	}
	f, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}

// sample returns the colour at p, or false if p is outside the icon.
func sample(x, y float64) ([3]float64, bool) {
	if !inRoundRect(x, y, 100, 100, 924, 924, 185) {
		return [3]float64{}, false
	}
	// Diagonal violet gradient, as in MP3 Renamer.
	t := (x + y) / (2 * size)
	bg := [3]float64{lerp(0x9a, 0x55, t), lerp(0x7c, 0x33, t), lerp(0xff, 0xe0, t)}

	r := math.Hypot(x-512, y-512)
	switch {
	case r <= 22: // spindle hole
		return bg, true
	case r <= 118: // label
		return white, true
	case r <= 136:
		return disc, true
	case r <= 330:
		if int((r-136)/18)%2 == 1 {
			return groove, true
		}
		return disc, true
	}
	return bg, true
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func inRoundRect(x, y, x0, y0, x1, y1, rad float64) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	cx := math.Max(math.Max(x0+rad-x, x-(x1-rad)), 0)
	cy := math.Max(math.Max(y0+rad-y, y-(y1-rad)), 0)
	return cx*cx+cy*cy <= rad*rad
}
