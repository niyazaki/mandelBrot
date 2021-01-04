package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
)

var (
	port    int
	maxIter int
)

func init() {
	flag.IntVar(&maxIter, "maxIter", 30, "Specified port")
}

func main() {
	port := os.Args[1]
	fmt.Println("Slave listening at port " + port)

	http.HandleFunc("/", mandelHTTP)
	http.ListenAndServe(":"+port, nil)
}

func mandelHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	q := req.URL.Query()
	x, err1 := strconv.ParseFloat(q.Get("x"), 64)
	y, err2 := strconv.ParseFloat(q.Get("y"), 64)
	if err1 != nil || err2 != nil {
		http.NotFound(w, req)
		return
	}
	_, iter := mandelIteration(x, y, maxIter)
	data, err := json.Marshal(iter)
	if err != nil {
		panic(err)
	}
	w.Write(data)
	// log.Println(x, y, iter)
}

//Mandelbrot function z_{n+1} = z_n^2 + c
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
