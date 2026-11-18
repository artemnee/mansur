// Package brain is a tiny GPT (a decoder-only transformer, ~470K parameters)
// written from scratch: forward and backward pass, training, generation.
//
// Character-level sequence, with memory of past exchanges:
// [mood] question [SEP] answer [TURN] question [SEP] answer [END]
package brain

import (
	"math"
	"math/rand/v2"
)

type Config struct {
	C, L, NH, T int // width, layers, attention heads, context length
}

type layer struct {
	ln1w, ln1b, qkvw, qkvb, projw, projb, ln2w, ln2b, fcw, fcb, fc2w, fc2b []float32
}

type Model struct {
	Config
	Moods []string
	Chars []rune

	wte, wpe   []float32 // token embeddings (also the output layer) and position embeddings
	layers     []layer
	lnfw, lnfb []float32

	charIdx map[rune]int
}

// Tokens: 0..M-1 are moods, M is SEP, M+1 is END, M+2 is TURN, then characters.
func (m *Model) V() int    { return len(m.Moods) + 3 + len(m.Chars) }
func (m *Model) sep() int  { return len(m.Moods) }
func (m *Model) end() int  { return len(m.Moods) + 1 }
func (m *Model) turn() int { return len(m.Moods) + 2 }
func (m *Model) base() int { return len(m.Moods) + 3 }

// Turn is one past exchange: what the human wrote and what Mansur answered.
type Turn struct{ Q, A string }

// prompt builds the input: mood, past exchanges (as many as fit) and the new question.
func (m *Model) prompt(mood string, history []Turn, text string, room int) (toks []int, from int) {
	md := 0
	for i, x := range m.Moods {
		if x == mood {
			md = i
		}
	}
	clip := func(t []int) []int { return t[:min(len(t), MaxQ)] }
	q := append(clip(m.encode(text)), m.sep())
	var ctx []int
	for i := len(history) - 1; i >= 0; i-- {
		h := append(append(clip(m.encode(history[i].Q)), m.sep()), clip(m.encode(history[i].A))...)
		h = append(h, m.turn())
		if 1+len(h)+len(ctx)+len(q)+room > m.T {
			break
		}
		ctx = append(h, ctx...)
	}
	toks = append(append([]int{md}, ctx...), q...)
	return toks, len(toks) - 1
}

func newModel(cfg Config, moods []string, chars []rune) *Model {
	m := &Model{Config: cfg, Moods: moods, Chars: chars}
	C, V := cfg.C, m.V()
	f := func(n int) []float32 { return make([]float32, n) }
	m.wte, m.wpe = f(V*C), f(cfg.T*C)
	for range cfg.L {
		m.layers = append(m.layers, layer{
			ln1w: f(C), ln1b: f(C), qkvw: f(3 * C * C), qkvb: f(3 * C), projw: f(C * C), projb: f(C),
			ln2w: f(C), ln2b: f(C), fcw: f(4 * C * C), fcb: f(4 * C), fc2w: f(4 * C * C), fc2b: f(C),
		})
	}
	m.lnfw, m.lnfb = f(C), f(C)
	m.charIdx = map[rune]int{}
	for i, c := range chars {
		m.charIdx[c] = m.base() + i
	}
	return m
}

func (m *Model) params() [][]float32 {
	ps := [][]float32{m.wte, m.wpe}
	for _, l := range m.layers {
		ps = append(ps, l.ln1w, l.ln1b, l.qkvw, l.qkvb, l.projw, l.projb, l.ln2w, l.ln2b, l.fcw, l.fcb, l.fc2w, l.fc2b)
	}
	return append(ps, m.lnfw, m.lnfb)
}

func (m *Model) NumParams() int {
	n := 0
	for _, p := range m.params() {
		n += len(p)
	}
	return n
}

// encode turns the human's text into tokens, dropping unknown characters.
func (m *Model) encode(s string) []int {
	var out []int
	for _, r := range s {
		if i, ok := m.charIdx[r]; ok {
			out = append(out, i)
		}
	}
	return out
}

// acts holds every intermediate value of one sequence's forward pass (needed for the backward pass).
type acts struct {
	n                                   int
	x0                                  []float32
	ln1, ln1m, ln1r, qkv, att, atty, x1 [][]float32
	ln2, ln2m, ln2r, fch, fchg, x2      [][]float32
	lnf, lnfm, lnfr, probs              []float32
}

func (m *Model) newActs() *acts {
	C, T, NH, L, V := m.C, m.T, m.NH, m.L, m.V()
	f := func(n int) []float32 { return make([]float32, n) }
	ff := func(n int) [][]float32 {
		s := make([][]float32, L)
		for i := range s {
			s[i] = f(n)
		}
		return s
	}
	return &acts{
		x0:  f(T * C),
		ln1: ff(T * C), ln1m: ff(T), ln1r: ff(T), qkv: ff(T * 3 * C), att: ff(NH * T * T), atty: ff(T * C), x1: ff(T * C),
		ln2: ff(T * C), ln2m: ff(T), ln2r: ff(T), fch: ff(T * 4 * C), fchg: ff(T * 4 * C), x2: ff(T * C),
		lnf: f(T * C), lnfm: f(T), lnfr: f(T), probs: f(T * V),
	}
}

func layernorm(out, mean, rstd, inp, w, b []float32, n, C int) {
	for t := range n {
		x := inp[t*C : (t+1)*C]
		var mu, v float32
		for _, a := range x {
			mu += a
		}
		mu /= float32(C)
		for _, a := range x {
			v += (a - mu) * (a - mu)
		}
		r := 1 / float32(math.Sqrt(float64(v/float32(C)+1e-5)))
		o := out[t*C : (t+1)*C]
		for i := range C {
			o[i] = (x[i]-mu)*r*w[i] + b[i]
		}
		mean[t], rstd[t] = mu, r
	}
}

func layernormBack(dinp, dw, db, dout, inp, w, mean, rstd []float32, n, C int) {
	for t := range n {
		d, x, di := dout[t*C:(t+1)*C], inp[t*C:(t+1)*C], dinp[t*C:(t+1)*C]
		var dm, dmn float32
		for i := range C {
			norm := (x[i] - mean[t]) * rstd[t]
			dn := w[i] * d[i]
			dm += dn
			dmn += dn * norm
		}
		dm /= float32(C)
		dmn /= float32(C)
		for i := range C {
			norm := (x[i] - mean[t]) * rstd[t]
			db[i] += d[i]
			dw[i] += norm * d[i]
			di[i] += (w[i]*d[i] - dm - norm*dmn) * rstd[t]
		}
	}
}

// matmul: out[t][o] = b[o] + Σ inp[t][i]·w[o][i]
func matmul(out, inp, w, b []float32, n, in, outN int) {
	for t := range n {
		x := inp[t*in : (t+1)*in]
		for o := range outN {
			row := w[o*in : (o+1)*in]
			s := float32(0)
			if b != nil {
				s = b[o]
			}
			for i, a := range x {
				s += a * row[i]
			}
			out[t*outN+o] = s
		}
	}
}

func matmulBack(dinp, dw, db, dout, inp, w []float32, n, in, outN int) {
	for t := range n {
		x, dx := inp[t*in:(t+1)*in], dinp[t*in:(t+1)*in]
		for o := range outN {
			d := dout[t*outN+o]
			if d == 0 {
				continue
			}
			row, drow := w[o*in:(o+1)*in], dw[o*in:(o+1)*in]
			for i := range in {
				dx[i] += d * row[i]
				drow[i] += d * x[i]
			}
			if db != nil {
				db[o] += d
			}
		}
	}
}

const geluK = 0.7978845608 // sqrt(2/π)

func gelu(out, inp []float32) {
	for i, x := range inp {
		out[i] = 0.5 * x * (1 + float32(math.Tanh(float64(geluK*(x+0.044715*x*x*x)))))
	}
}

func geluBack(dinp, inp, dout []float32) {
	for i, x := range inp {
		u := geluK * (x + 0.044715*x*x*x)
		th := float32(math.Tanh(float64(u)))
		sech2 := 1 - th*th
		dinp[i] += (0.5*(1+th) + 0.5*x*sech2*geluK*(1+3*0.044715*x*x)) * dout[i]
	}
}

func (m *Model) attention(out, att, qkv []float32, n int) {
	C, NH, T := m.C, m.NH, m.T
	hs := C / NH
	scale := 1 / float32(math.Sqrt(float64(hs)))
	for t := range n {
		for h := range NH {
			q := qkv[t*3*C+h*hs : t*3*C+(h+1)*hs]
			a := att[(h*T+t)*T : (h*T+t)*T+T]
			mx := float32(-1e30)
			for t2 := 0; t2 <= t; t2++ {
				k := qkv[t2*3*C+C+h*hs : t2*3*C+C+(h+1)*hs]
				a[t2] = dot(q, k) * scale
				mx = max(mx, a[t2])
			}
			var sum float32
			for t2 := 0; t2 <= t; t2++ {
				a[t2] = float32(math.Exp(float64(a[t2] - mx)))
				sum += a[t2]
			}
			o := out[t*C+h*hs : t*C+(h+1)*hs]
			clear(o)
			for t2 := 0; t2 <= t; t2++ {
				a[t2] /= sum
				v := qkv[t2*3*C+2*C+h*hs : t2*3*C+2*C+(h+1)*hs]
				for i := range hs {
					o[i] += a[t2] * v[i]
				}
			}
		}
	}
}

func (m *Model) attentionBack(dqkv, dout, att, qkv []float32, n int) {
	C, NH, T := m.C, m.NH, m.T
	hs := C / NH
	scale := 1 / float32(math.Sqrt(float64(hs)))
	datt := make([]float32, T)
	for t := range n {
		for h := range NH {
			a := att[(h*T+t)*T : (h*T+t)*T+T]
			d := dout[t*C+h*hs : t*C+(h+1)*hs]
			var s float32
			for t2 := 0; t2 <= t; t2++ {
				v := qkv[t2*3*C+2*C+h*hs : t2*3*C+2*C+(h+1)*hs]
				dv := dqkv[t2*3*C+2*C+h*hs : t2*3*C+2*C+(h+1)*hs]
				datt[t2] = dot(v, d)
				for i := range hs {
					dv[i] += a[t2] * d[i]
				}
				s += a[t2] * datt[t2]
			}
			q := qkv[t*3*C+h*hs : t*3*C+(h+1)*hs]
			dq := dqkv[t*3*C+h*hs : t*3*C+(h+1)*hs]
			for t2 := 0; t2 <= t; t2++ {
				dpre := a[t2] * (datt[t2] - s) * scale
				k := qkv[t2*3*C+C+h*hs : t2*3*C+C+(h+1)*hs]
				dk := dqkv[t2*3*C+C+h*hs : t2*3*C+C+(h+1)*hs]
				for i := range hs {
					dq[i] += dpre * k[i]
					dk[i] += dpre * q[i]
				}
			}
		}
	}
}

func dot(a, b []float32) float32 {
	var s float32
	for i := range b {
		s += a[i] * b[i]
	}
	return s
}

// forward computes next-token probabilities for every position into a.probs.
func (m *Model) forward(toks []int, a *acts) {
	C, V := m.C, m.V()
	n := len(toks)
	a.n = n
	for t, tok := range toks {
		for i := range C {
			a.x0[t*C+i] = m.wte[tok*C+i] + m.wpe[t*C+i]
		}
	}
	x := a.x0
	for li, l := range m.layers {
		layernorm(a.ln1[li], a.ln1m[li], a.ln1r[li], x, l.ln1w, l.ln1b, n, C)
		matmul(a.qkv[li], a.ln1[li], l.qkvw, l.qkvb, n, C, 3*C)
		m.attention(a.atty[li], a.att[li], a.qkv[li], n)
		matmul(a.x1[li], a.atty[li], l.projw, l.projb, n, C, C)
		for i := range n * C {
			a.x1[li][i] += x[i]
		}
		layernorm(a.ln2[li], a.ln2m[li], a.ln2r[li], a.x1[li], l.ln2w, l.ln2b, n, C)
		matmul(a.fch[li], a.ln2[li], l.fcw, l.fcb, n, C, 4*C)
		gelu(a.fchg[li][:n*4*C], a.fch[li][:n*4*C])
		matmul(a.x2[li], a.fchg[li], l.fc2w, l.fc2b, n, 4*C, C)
		for i := range n * C {
			a.x2[li][i] += a.x1[li][i]
		}
		x = a.x2[li]
	}
	layernorm(a.lnf, a.lnfm, a.lnfr, x, m.lnfw, m.lnfb, n, C)
	matmul(a.probs, a.lnf, m.wte, nil, n, C, V)
	for t := range n {
		p := a.probs[t*V : (t+1)*V]
		mx := p[0]
		for _, v := range p {
			mx = max(mx, v)
		}
		var sum float32
		for i := range p {
			p[i] = float32(math.Exp(float64(p[i] - mx)))
			sum += p[i]
		}
		for i := range p {
			p[i] /= sum
		}
	}
}

// Reply is Mansur's answer to the human's text, given past exchanges (an empty text means Mansur
// talks on his own, by mood). conf is the mean probability of the chosen characters:
// the lower it is, the more the network was guessing.
func (m *Model) Reply(mood string, history []Turn, text string, temp float64, rng *rand.Rand) (reply string, conf float64) {
	toks, _ := m.prompt(mood, history, text, 40)
	c := m.newCache()
	var p []float32
	for _, t := range toks {
		p = m.push(c, t)
	}
	V := m.V()
	w := make([]float64, V)
	var out []rune
	var logp float64
	n := 0
	for c.n < m.T {
		clear(w)
		var sum float64
		for v := m.end(); v < V; v++ {
			if v == m.turn() {
				continue
			}
			w[v] = math.Pow(float64(p[v]), 1/temp)
			sum += w[v]
		}
		pick, acc := m.end(), rng.Float64()*sum
		for v := m.end(); v < V; v++ {
			if acc -= w[v]; acc <= 0 && w[v] > 0 {
				pick = v
				break
			}
		}
		logp += math.Log(float64(p[pick]) + 1e-9)
		n++
		if pick == m.end() {
			break
		}
		out = append(out, m.Chars[pick-m.base()])
		if c.n < m.T {
			p = m.push(c, pick)
		}
	}
	return string(out), math.Exp(logp / float64(n))
}

const MaxQ = 32
