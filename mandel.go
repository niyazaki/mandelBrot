package main

// Authors : Kassabeh Zakariya
// 			 El Bakkoury Yassine
//			 Endre Simo

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"palette"
)

var (
	colorPalette     string
	colorStep        float64
	xmin, ymin       float64
	width, height    int
	imageSmoothness  int
	maxIteration     int
	outputFile       string
	loadBalencerURL  string
	loadBalencerPort string
	mode             string
)

var waitGroup sync.WaitGroup

func init() {
	flag.Float64Var(&colorStep, "step", 6000, "Color smooth step. Value should be greater than iteration count, otherwise the value will be adjusted to the iteration count.")
	flag.IntVar(&width, "width", 1280, "Rendered image width")
	flag.IntVar(&height, "height", 720, "Rendered image height")
	flag.Float64Var(&xmin, "xmin", -2.1, "Point position on the real axis (defined on `x` axis)")
	flag.Float64Var(&ymin, "ymin", -1.2, "Point position on the imaginary axis (defined on `y` axis)")
	flag.IntVar(&maxIteration, "maxIter", 800, "Iteration count")
	flag.IntVar(&imageSmoothness, "smoothness", 8, "The rendered mandelbrot set smoothness. For a more detailded and clear image use higher numbers. For 4xAA (AA = antialiasing) use -smoothness 4")
	flag.StringVar(&colorPalette, "palette", "Hippi", "Hippi | Plan9 | AfternoonBlue | SummerBeach | Biochimist | Fiesta")
	flag.StringVar(&outputFile, "file", "mandelbrot.png", "The rendered mandelbrot image filname")
	flag.StringVar(&loadBalencerURL, "lbURL", "http://localhost", "The load balancer URL")
	flag.StringVar(&loadBalencerPort, "lbPort", "3030", "The load balancer port")
	flag.StringVar(&mode, "mode", "verticalOpti", "The mode you want to run the project with : simple | simpleOpti | horizontal | vertical | verticalOpti")
	flag.Parse()
}

func main() {
	start := time.Now()

	done := make(chan struct{})
	ticker := time.NewTicker(time.Millisecond * 100)

	go func() {
		for {
			select {
			case <-ticker.C:
				fmt.Print(".")
			case <-done:
				ticker.Stop()
				fmt.Printf("\n\nMandelbrot set rendered into `%s`\n", outputFile)
			}
		}
	}()

	if colorStep < float64(maxIteration) {
		colorStep = float64(maxIteration)
	}
	colors := interpolateColors(&colorPalette, colorStep)

	if len(colors) > 0 {
		fmt.Print("Rendering image...")
		switch {
		case mode == "simple":
			smoothColoringSingle(maxIteration, colors)

		case mode == "simpleOpti":
			renderSingle(maxIteration, colors)

		case mode == "horizontal":
			renderHTTP(maxIteration, colors, done)

		case mode == "vertical":
			smoothColoring(maxIteration, colors, done)

		case mode == "verticalOpti":
			render(maxIteration, colors, done)
		}

	}

	elapsed := time.Since(start)
	log.Printf("Process mandelbrot took %s", elapsed)

	time.Sleep(time.Second)
}

func interpolateColors(paletteCode *string, numberOfColors float64) []color.RGBA {
	var factor float64
	steps := []float64{}
	cols := []uint32{}
	interpolated := []uint32{}
	interpolatedColors := []color.RGBA{}

	for _, v := range palette.ColorPalettes {
		factor = 1.0 / numberOfColors
		switch v.Keyword {
		case *paletteCode:
			if paletteCode != nil {
				for index, col := range v.Colors {
					if col.Step == 0.0 && index != 0 {
						stepRatio := float64(index+1) / float64(len(v.Colors))
						step := float64(int(stepRatio*100)) / 100 // truncate to 2 decimal precision
						steps = append(steps, step)
					} else {
						steps = append(steps, col.Step)
					}
					r, g, b, a := col.Color.RGBA()
					r /= 0xff
					g /= 0xff
					b /= 0xff
					a /= 0xff
					uintColor := uint32(r)<<24 | uint32(g)<<16 | uint32(b)<<8 | uint32(a)
					cols = append(cols, uintColor)
				}

				var min, max, minColor, maxColor float64
				if len(v.Colors) == len(steps) && len(v.Colors) == len(cols) {
					for i := 0.0; i <= 1; i += factor {
						for j := 0; j < len(v.Colors)-1; j++ {
							if i >= steps[j] && i < steps[j+1] {
								min = steps[j]
								max = steps[j+1]
								minColor = float64(cols[j])
								maxColor = float64(cols[j+1])
								uintColor := cosineInterpolation(maxColor, minColor, (i-min)/(max-min))
								interpolated = append(interpolated, uint32(uintColor))
							}
						}
					}
				}

				for _, pixelValue := range interpolated {
					r := pixelValue >> 24 & 0xff
					g := pixelValue >> 16 & 0xff
					b := pixelValue >> 8 & 0xff
					a := 0xff

					interpolatedColors = append(interpolatedColors, color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)})
				}
			}
		}
	}

	return interpolatedColors
}

// Optimized concurrency rendering
func render(maxIteration int, colors []color.RGBA, done chan struct{}) {
	width = width * imageSmoothness //Image resolution
	height = height * imageSmoothness
	xmax := math.Abs(xmin)
	ymax := math.Abs(ymin)

	image := image.NewRGBA(image.Rectangle{image.Point{0, 0}, image.Point{width, height}})

	for iy := 0; iy < height; iy++ {
		waitGroup.Add(1)
		go func(iy int) {
			defer waitGroup.Done()

			for ix := 0; ix < width; ix++ {
				var x = xmin + (xmax-xmin)*float64(ix)/float64(width-1)
				var y = ymin + (ymax-ymin)*float64(iy)/float64(height-1)

				norm, it := mandelIteration(x, y, maxIteration)
				iteration := float64(maxIteration-it) + math.Log(norm)

				if int(math.Abs(iteration)) < len(colors)-1 {
					color1 := colors[int(math.Abs(iteration))]
					color2 := colors[int(math.Abs(iteration))+1]
					color := linearInterpolation(rgbaToUint(color1), rgbaToUint(color2), uint32(iteration))
					// color := linearInterpolation2(rgbaToUint(color1), rgbaToUint(color2), iteration)

					image.Set(ix, iy, uint32ToRgba(color))
				}
			}
		}(iy)
	}

	waitGroup.Wait()
	output, _ := os.Create(outputFile)
	png.Encode(output, image)

	done <- struct{}{}
}

// Horizontal rendering
func renderHTTP(maxIteration int, colors []color.RGBA, done chan struct{}) {
	width = width * imageSmoothness
	height = height * imageSmoothness
	xmax := math.Abs(xmin)
	ymax := math.Abs(ymin)
	var myClient = &http.Client{Timeout: 10 * time.Second}

	image := image.NewRGBA(image.Rectangle{image.Point{0, 0}, image.Point{width, height}})

	for iy := 0; iy < height; iy++ {
		waitGroup.Add(1)
		go func(iy int) {
			defer waitGroup.Done()

			for ix := 0; ix < width; ix++ {
				var x = xmin + (xmax-xmin)*float64(ix)/float64(width-1)
				var y = ymin + (ymax-ymin)*float64(iy)/float64(height-1)
				var url = loadBalencerURL + ":" + loadBalencerPort + "/?x=" + fmt.Sprintf("%f", x) + "&y=" + fmt.Sprintf("%f", y)

				resp, err := myClient.Get(url)
				if err != nil {
					panic(err)
				}
				defer resp.Body.Close()

				content, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					panic(err)
				}

				iter, _ := strconv.Atoi(string(content))
				// time.Sleep(time.Millisecond)

				// if iter < len(colors)-1 {
				// 	color1 := colors[iter]
				// 	color2 := colors[iter+1]
				// 	color := linearInterpolation(rgbaToUint(color1), rgbaToUint(color2), uint32(iter))

				// 	image.Set(ix, iy, uint32ToRgba(color))
				// }
				image.Set(ix, iy, colors[iter])
			}
		}(iy)
	}

	waitGroup.Wait()
	outputFile = strings.Replace(outputFile, ".png", "Horizontal.png", 1)
	output, _ := os.Create(outputFile)
	png.Encode(output, image)

	done <- struct{}{}
}

// Concurrency rendering using the smooth coloring algorithm found at :
// https://www.wikiwand.com/en/Plotting_algorithms_for_the_Mandelbrot_set#/Continuous_(smooth)_coloring
func smoothColoring(maxIteration int, colors []color.RGBA, done chan struct{}) {
	width = width * imageSmoothness
	height = height * imageSmoothness
	xmax := math.Abs(xmin)
	ymax := math.Abs(ymin)

	image := image.NewRGBA(image.Rectangle{image.Point{0, 0}, image.Point{width, height}})
	for iy := 0; iy < height; iy++ {
		waitGroup.Add(1)
		go func(iy int) {
			defer waitGroup.Done()
			var iteration float64 = 0.0
			for ix := 0; ix < width; ix++ {
				var x = xmin + (xmax-xmin)*float64(ix)/float64(width-1)
				var y = ymin + (ymax-ymin)*float64(iy)/float64(height-1)
				logZn, it := mandelIterSmooth(x, y, maxIteration)
				if it < maxIteration {
					nu := math.Log(logZn/math.Log(2)) / math.Log(2)
					// Rearranging the potential function.
					// Dividing log_zn by log(2) instead of log(N = 1<<8)
					// because we want the entire palette to range from the
					// center to radius 2, NOT our bailout radius.

					iteration = math.Abs(float64(it) + 1.0 - nu)

					index := int(math.Floor(iteration))
					color1 := colors[index]
					color2 := colors[index+1]
					color := linearInterpolation2(rgbaToUint(color1), rgbaToUint(color2), iteration)
					image.Set(ix, iy, uint32ToRgba(color))
				}

			}
		}(iy)
	}
	waitGroup.Wait()
	outputFile = strings.Replace(outputFile, ".png", "Smooth.png", 1)
	output, _ := os.Create(outputFile)
	png.Encode(output, image)

	done <- struct{}{}
}

// Single-thread rendering using the smooth coloring algorithm found at :
// https://www.wikiwand.com/en/Plotting_algorithms_for_the_Mandelbrot_set#/Continuous_(smooth)_coloring
func smoothColoringSingle(maxIteration int, colors []color.RGBA) {
	width = width * imageSmoothness
	height = height * imageSmoothness
	xmax := math.Abs(xmin)
	ymax := math.Abs(ymin)

	image := image.NewRGBA(image.Rectangle{image.Point{0, 0}, image.Point{width, height}})
	for iy := 0; iy < height; iy++ {
		var iteration float64 = 0.0
		for ix := 0; ix < width; ix++ {
			var x = xmin + (xmax-xmin)*float64(ix)/float64(width-1)
			var y = ymin + (ymax-ymin)*float64(iy)/float64(height-1)
			logZn, it := mandelIterSmooth(x, y, maxIteration)
			if it < maxIteration {
				nu := math.Log(logZn/math.Log(2)) / math.Log(2)
				// Rearranging the potential function.
				// Dividing log_zn by log(2) instead of log(N = 1<<8)
				// because we want the entire palette to range from the
				// center to radius 2, NOT our bailout radius.

				iteration = math.Abs(float64(it) + 1.0 - nu)

				index := int(math.Floor(iteration))
				color1 := colors[index]
				color2 := colors[index+1]
				color := linearInterpolation2(rgbaToUint(color1), rgbaToUint(color2), iteration)
				image.Set(ix, iy, uint32ToRgba(color))
			}

		}
	}
	outputFile = strings.Replace(outputFile, ".png", "Simple.png", 1)
	output, _ := os.Create(outputFile)
	png.Encode(output, image)
}

// // Single threaded rendering using the histogram coloring algorithm found at :
// // https://www.wikiwand.com/en/Plotting_algorithms_for_the_Mandelbrot_set#/Histogram_coloring
// func histogramColoring(maxIter int) map[[2]float64]float64 {
// 	width = width * imageSmoothness
// 	height = height * imageSmoothness
// 	xmax := math.Abs(xmin)
// 	ymax := math.Abs(ymin)

// 	totalIterations := 0
// 	iterationCounts := make(map[[2]float64]int)
// 	numIterationsPerPixel := make([]int, maxIter+1)
// 	hue := make(map[[2]float64]float64)

// 	image := image.NewRGBA(image.Rectangle{image.Point{0, 0}, image.Point{width, height}})
// 	//First pass
// 	for iy := 0; iy < height; iy++ {
// 		for ix := 0; ix < width; ix++ {
// 			var x = xmin + (xmax-xmin)*float64(ix)/float64(width-1)
// 			var y = ymin + (ymax-ymin)*float64(iy)/float64(height-1)
// 			_, iteration := mandelIteration(x, y, maxIter)
// 			iterationCounts[[2]float64{x, y}] = iteration
// 			//Second pass
// 			numIterationsPerPixel[iteration]++
// 		}
// 	}

// 	//Third pass
// 	for i := 0; i < len(numIterationsPerPixel); i++ {
// 		totalIterations += numIterationsPerPixel[i]
// 	}

// 	//Fourth pass
// 	for iy := 0; iy < height; iy++ {
// 		for ix := 0; ix < width; ix++ {
// 			var x = xmin + (xmax-xmin)*float64(ix)/float64(width-1)
// 			var y = ymin + (ymax-ymin)*float64(iy)/float64(height-1)
// 			iter := iterationCounts[[2]float64{x, y}]
// 			for i := 0; i <= int(iter); i++ {
// 				colorValue := float64(numIterationsPerPixel[i]) / float64(totalIterations)
// 				hue[[2]float64{x, y}] += colorValue
// 			}
// 			image.Set(ix, iy, uint32ToRgba(uint32(hue[[2]float64{x, y}])))
// 		}
// 	}
// 	outputFile = strings.Replace(outputFile, ".png", "SimpleHistogram.png", 1)
// 	output, _ := os.Create(outputFile)
// 	png.Encode(output, image)
// 	return hue
// }

// Optimized single-threaded rendering
func renderSingle(maxIteration int, colors []color.RGBA) {
	width = width * imageSmoothness
	height = height * imageSmoothness
	xmax := math.Abs(xmin)
	ymax := math.Abs(ymin)

	image := image.NewRGBA(image.Rectangle{image.Point{0, 0}, image.Point{width, height}})

	for iy := 0; iy < height; iy++ {
		for ix := 0; ix < width; ix++ {
			var x = xmin + (xmax-xmin)*float64(ix)/float64(width-1)
			var y = ymin + (ymax-ymin)*float64(iy)/float64(height-1)

			norm, it := mandelIteration(x, y, maxIteration)
			iteration := float64(maxIteration-it) + math.Log(norm)

			if int(math.Abs(iteration)) < len(colors)-1 {
				color1 := colors[int(math.Abs(iteration))]
				color2 := colors[int(math.Abs(iteration))+1]
				color := linearInterpolation(rgbaToUint(color1), rgbaToUint(color2), uint32(iteration))
				// color := linearInterpolation2(rgbaToUint(color1), rgbaToUint(color2), iteration)

				image.Set(ix, iy, uint32ToRgba(color))
			}
		}
	}
	outputFile = strings.Replace(outputFile, ".png", "SimpleOpti.png", 1)
	output, _ := os.Create(outputFile)
	png.Encode(output, image)
}

// Mandelbrot function but for smooth rendering algorithm
func mandelIterSmooth(cx, cy float64, maxIter int) (float64, int) {
	var x, y, x2, y2, xy2 float64 = 0.0, 0.0, 0.0, 0.0, 0.0
	var iter int = 0
	for i := 0; i < maxIter && x2+y2 < 1<<16; i++ {
		xy2 = (x + x) * y //optimised 2*x*y to reduce the number of multiplications
		x2 = x * x
		y2 = y * y

		x = x2 - y2 + cx
		y = xy2 + cy

		iter = i
	}
	logZn := (x*x + y*y) / 2
	return logZn, iter
}

//Mandelbrot function
func mandelIteration(cx, cy float64, maxIter int) (float64, int) {
	var x, y, x2, y2, xy2 float64 = 0.0, 0.0, 0.0, 0.0, 0.0

	for i := 0; i < maxIter; i++ {
		xy2 = (x + x) * y
		x2 = x * x
		y2 = y * y
		if x2+y2 > 4 {
			return x2 + y2, i
		}
		x = x2 - y2 + cx
		y = xy2 + cy //optimised 2*x*y to reduce the number of multiplications
	}

	logZn := (x*x + y*y) / 2
	return logZn, maxIter
}

func rgbaToUint(color color.RGBA) uint32 {
	r, g, b, a := color.RGBA()
	r /= 0xff
	g /= 0xff
	b /= 0xff
	a /= 0xff
	return uint32(r)<<24 | uint32(g)<<16 | uint32(b)<<8 | uint32(a)
}

func uint32ToRgba(col uint32) color.RGBA {
	r := col >> 24 & 0xff
	g := col >> 16 & 0xff
	b := col >> 8 & 0xff
	a := 0xff
	return color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
}

func cosineInterpolation(c1, c2, mu float64) float64 {
	mu2 := (1 - math.Cos(mu*math.Pi)) / 2.0
	return c1*(1-mu2) + c2*mu2
}

func linearInterpolation(c1, c2, mu uint32) uint32 {
	return c1*(1-mu) + c2*mu
}

func linearInterpolation2(c1, c2 uint32, mu float64) uint32 {
	return uint32(float64(c1)*(1-mu) + float64(c2)*mu)
}
