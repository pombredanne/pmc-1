package pmc

import (
	"errors"
	"fmt"
	"math"
	"strconv"

	"code.google.com/p/gofarmhash"

	"github.com/lazybeaver/xorshift"
	"github.com/lukut/bitmaps"
)

var xor64s = xorshift.NewXorShift64Star(42)

// non-receiver methods

func georand(w uint) uint {
	val := uint(xor64s.Next())
	// Calculate the position of the leftmost 1-bit.
	for r := uint(0); r < w-1; r++ {
		if val&0x8000000000000000 != 0 {
			return r
		}
		val <<= 1
	}
	return w
}

func rand(m uint) uint {
	return uint(xor64s.Next()) % m
}

func qk(k, n, p float64) float64 {
	result := 1.0
	for i := 1.0; i <= k; i++ {
		result *= (1.0 - math.Pow(1.0-math.Pow(2, -i), n)*(1.0-p))
	}
	return result
}

/*
Sketch is a Probabilistic Multiplicity Counting Sketch, a novel data structure
that is capable of accounting traffic per flow probabilistically, that can be
used as an alternative to Count-min sketch.
*/
type Sketch struct {
	l      float64
	m      float64
	w      float64
	bitmap bitmaps.Bitmap // FIXME: Get Rid of bitmap and use uint32 array
}

/*
New returns a PMC Sketch with the properties:
l = total number of bits for sketch
m = total number of rows for each flow
w = total number of columns for each flow
*/
func New(l uint, m uint, w uint) (*Sketch, error) {
	if l == 0 {
		return nil, errors.New("Expected l > 0, got 0")
	}
	if m == 0 {
		return nil, errors.New("Expected m > 0, got 0")
	}
	if w == 0 {
		return nil, errors.New("Expected w > 0, got 0")
	}
	return &Sketch{float64(l), float64(m), float64(w), make(bitmaps.Bitmap, l/8)}, nil
}

/*
NewForMaxFlows returns a PMC Sketch adapted to the size of the max number of
flows expected.
*/
func NewForMaxFlows(maxFlows uint) (*Sketch, error) {
	l := maxFlows * 32
	return New(l, 256, 32)
}

func (sketch *Sketch) printVirtualMatrix(flow []byte) {
	for i := 0.0; i < sketch.m; i++ {
		for j := 0.0; j < sketch.w; j++ {
			pos := sketch.getPos(flow, i, j)
			if sketch.bitmap.Get(pos) == false {
				fmt.Print(0)
			} else {
				fmt.Print(1)
			}
		}
		fmt.Println("")
	}
}

/*
GetFillRate ...
*/
func (sketch *Sketch) GetFillRate() float64 {
	return sketch.getP() * 100
}

/*
It is straightforward to use any uniformly distributed hash function with
sufficiently random output in the role of H: the input parameters can
simply be concatenated to a single bit string.
*/
func (sketch *Sketch) getPos(f []byte, i, j float64) uint {
	s := strconv.Itoa(int(i)) + string(f[len(f)/2:]) + strconv.Itoa(int(j)) + string(f[:len(f)])
	hash := farmhash.Hash64([]byte(s))
	return uint(hash) % uint(sketch.l)
}

/*
Increment the count of the flow by 1
*/
func (sketch *Sketch) Increment(flow []byte) {
	i := rand(uint(sketch.m))
	j := georand(uint(sketch.w))
	pos := sketch.getPos(flow, float64(i), float64(j))
	sketch.bitmap.Set(pos, true)
}

func (sketch *Sketch) getZSum(flow []byte) float64 {
	z := 0.0
	for i := 0.0; i < sketch.m; i++ {
		j := 0.0
		for j < sketch.w {
			pos := sketch.getPos(flow, i, j)
			if sketch.bitmap.Get(pos) == false {
				break
			}
			j++
		}
		z += j
	}
	return z
}

func (sketch *Sketch) getEmptyRows(flow []byte) float64 {
	k := 0.0
	for i := 0.0; i < sketch.m; i++ {
		pos := sketch.getPos(flow, i, 0)
		if sketch.bitmap.Get(pos) == false {
			k++
		}
	}
	return k
}

func (sketch *Sketch) getP() float64 {
	ones := 0.0
	for i := uint(0); i < uint(sketch.bitmap.Size()); i++ {
		if sketch.bitmap.Get(i) == true {
			ones++
		}
	}
	return ones / sketch.l
}

func (sketch *Sketch) getE(n, p float64) float64 {
	result := 0.0
	for k := 1.0; k <= sketch.w; k++ {
		result += (k * (qk(k, n, p) - qk(k+1, n, p)))
	}
	return result
}

func (sketch *Sketch) rho(n, p float64) float64 {
	return math.Pow(2, sketch.getE(n, p)) / n
}

/*
GetEstimate returns the estimated count of a given flow
*/
func (sketch *Sketch) GetEstimate(flow []byte) uint {
	p := sketch.getP()
	k := sketch.getEmptyRows(flow)
	// Use const due to quick conversion against 0.78 (n = 1000000.0)
	// n := -2 * sketch.m * math.Log(k) / (m * (1 - p))
	n := 100000.0

	// Dealing with small multiplicities
	if k/(1-p) > 0.3*sketch.m {
		return uint(-2 * sketch.m * math.Log(k/(sketch.m*(1-p))))
	}

	z := sketch.getZSum(flow)
	return uint(sketch.m * math.Pow(2, z/sketch.m) / sketch.rho(n, p))
}
