package brain

import (
	"math"
	"math/rand/v2"
	"runtime"
	"slices"
	"sync"
)

// Sample is a training example: mood, past exchanges, question and Mansur's answer.
type Sample struct {
	Mood    string
	History []Turn
	Q, A    string
}

type TrainConfig struct {
	Config
	Epochs int
	Batch  int
	LR     float64
	Log    func(epoch int, loss float64, m *Model)
}

type grads struct {
	m                                *Model
	dx, dres, dln, dqkv, datty, dfch []float32
	dfchg, dlogits                   []float32
}

func (m *Model) newGrads() *grads {
	C, T := m.C, m.T
	f := func(n int) []float32 { return make([]float32, n) }
	return &grads{m: newModel(m.Config, m.Moods, m.Chars), dx: f(T * C), dres: f(T * C), dln: f(T * C),
		dqkv: f(T * 3 * C), datty: f(T * C), dfch: f(T * 4 * C), dfchg: f(T * 4 * C), dlogits: f(T * m.V())}
}

// backprop runs the forward and backward pass of one sequence. The loss covers the answer only.
func (m *Model) backprop(toks []int, from int, a *acts, g *grads) (loss float64, count int) {
	m.forward(toks[:len(toks)-1], a)
	C, V, n := m.C, m.V(), len(toks)-1
	clear(g.dlogits)
	for t := from; t < n; t++ {
		p := a.probs[t*V : (t+1)*V]
		d := g.dlogits[t*V : (t+1)*V]
		copy(d, p)
		d[toks[t+1]] -= 1
		loss -= math.Log(float64(p[toks[t+1]]) + 1e-9)
		count++
	}
	gm := g.m
	clear(g.dln)
	matmulBack(g.dln, gm.wte, nil, g.dlogits, a.lnf, m.wte, n, C, V)
	dx := g.dx
	clear(dx)
	last := a.x0
	if m.L > 0 {
		last = a.x2[m.L-1]
	}
	layernormBack(dx, gm.lnfw, gm.lnfb, g.dln, last, m.lnfw, a.lnfm, a.lnfr, n, C)

	for li := m.L - 1; li >= 0; li-- {
		l, gl := m.layers[li], gm.layers[li]
		xin := a.x0
		if li > 0 {
			xin = a.x2[li-1]
		}
		// x2 = x1 + fc2(gelu(fc(ln2(x1))))
		copy(g.dres, dx)
		clear(g.dfchg)
		matmulBack(g.dfchg, gl.fc2w, gl.fc2b, dx, a.fchg[li], l.fc2w, n, 4*C, C)
		clear(g.dfch)
		geluBack(g.dfch[:n*4*C], a.fch[li][:n*4*C], g.dfchg[:n*4*C])
		clear(g.dln)
		matmulBack(g.dln, gl.fcw, gl.fcb, g.dfch, a.ln2[li], l.fcw, n, C, 4*C)
		layernormBack(g.dres, gl.ln2w, gl.ln2b, g.dln, a.x1[li], l.ln2w, a.ln2m[li], a.ln2r[li], n, C)
		// x1 = xin + proj(attn(ln1(xin)))
		copy(dx, g.dres)
		clear(g.datty)
		matmulBack(g.datty, gl.projw, gl.projb, g.dres, a.atty[li], l.projw, n, C, C)
		clear(g.dqkv)
		m.attentionBack(g.dqkv, g.datty, a.att[li], a.qkv[li], n)
		clear(g.dln)
		matmulBack(g.dln, gl.qkvw, gl.qkvb, g.dqkv, a.ln1[li], l.qkvw, n, C, 3*C)
		layernormBack(dx, gl.ln1w, gl.ln1b, g.dln, xin, l.ln1w, a.ln1m[li], a.ln1r[li], n, C)
	}
	for t := range n {
		for i := range C {
			gm.wte[toks[t]*C+i] += dx[t*C+i]
			gm.wpe[t*C+i] += dx[t*C+i]
		}
	}
	return loss, count
}

// Train trains the GPT from scratch (AdamW, cosine learning-rate decay).
// Seq is one tokenized example; the loss covers the predictions from position From on.
type Seq struct {
	Toks []int
	From int
}

// Prepare builds the vocabulary from the samples and tokenizes them. The Go trainer and the
// PyTorch trainer (fed by cmd/train -dump) both train on exactly these sequences.
func Prepare(samples []Sample, cfg Config, extraChars string) (*Model, []Seq) {
	var moods []string
	set := map[rune]bool{}
	for _, s := range samples {
		if !slices.Contains(moods, s.Mood) {
			moods = append(moods, s.Mood)
		}
		for _, r := range s.Q + s.A {
			set[r] = true
		}
		for _, h := range s.History {
			for _, r := range h.Q + h.A {
				set[r] = true
			}
		}
	}
	for _, r := range extraChars {
		set[r] = true
	}
	var chars []rune
	for r := range set {
		chars = append(chars, r)
	}
	slices.Sort(chars)
	m := newModel(cfg, moods, chars)
	seqs := make([]Seq, len(samples))
	for i, s := range samples {
		ans := m.encode(s.A)
		toks, from := m.prompt(s.Mood, s.History, s.Q, len(ans)+1)
		toks = append(toks, ans...)
		toks = append(toks, m.end())
		if len(toks) > m.T+1 {
			toks = toks[:m.T+1]
		}
		seqs[i] = Seq{toks, from}
	}
	return m, seqs
}

func Train(samples []Sample, cfg TrainConfig, extraChars string) *Model {
	m, seqs := Prepare(samples, cfg.Config, extraChars)
	rng := rand.New(rand.NewPCG(1, 2))
	ones := func(p []float32) {
		for j := range p {
			p[j] = 1
		}
	}
	for _, p := range m.params() {
		for j := range p {
			p[j] = float32(rng.NormFloat64() * 0.02)
		}
	}
	for _, l := range m.layers {
		ones(l.ln1w)
		ones(l.ln2w)
		for _, b := range [][]float32{l.ln1b, l.ln2b, l.qkvb, l.projb, l.fcb, l.fc2b} {
			clear(b)
		}
	}
	ones(m.lnfw)
	clear(m.lnfb)

	workers := runtime.NumCPU()
	gs := make([]*grads, workers)
	as := make([]*acts, workers)
	for i := range workers {
		gs[i], as[i] = m.newGrads(), m.newActs()
	}
	total := newModel(m.Config, m.Moods, m.Chars)
	opt := newAdam(m)
	steps := cfg.Epochs * ((len(seqs) + cfg.Batch - 1) / cfg.Batch)
	step := 0

	for ep := 1; ep <= cfg.Epochs; ep++ {
		rng.Shuffle(len(seqs), func(i, j int) { seqs[i], seqs[j] = seqs[j], seqs[i] })
		var epLoss float64
		var epN int
		for b := 0; b < len(seqs); b += cfg.Batch {
			batch := seqs[b:min(b+cfg.Batch, len(seqs))]
			losses, counts := make([]float64, workers), make([]int, workers)
			var wg sync.WaitGroup
			for w := range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for _, p := range gs[w].m.params() {
						clear(p)
					}
					for i := w; i < len(batch); i += workers {
						l, c := m.backprop(batch[i].Toks, batch[i].From, as[w], gs[w])
						losses[w] += l
						counts[w] += c
					}
				}()
			}
			wg.Wait()
			n := 0
			tp := total.params()
			for _, p := range tp {
				clear(p)
			}
			for w := range workers {
				epLoss += losses[w]
				n += counts[w]
				for pi, p := range gs[w].m.params() {
					for i, v := range p {
						tp[pi][i] += v
					}
				}
			}
			epN += n
			step++
			warm := min(1, float64(step)/100)
			lr := cfg.LR * warm * (0.1 + 0.9*0.5*(1+math.Cos(math.Pi*float64(step)/float64(steps))))
			opt.step(m, total, lr, 1/float32(n))
		}
		if cfg.Log != nil {
			cfg.Log(ep, epLoss/float64(epN), m)
		}
	}
	return m
}

type adam struct {
	m, v [][]float32
	t    int
}

func newAdam(model *Model) *adam {
	a := &adam{}
	for _, p := range model.params() {
		a.m = append(a.m, make([]float32, len(p)))
		a.v = append(a.v, make([]float32, len(p)))
	}
	return a
}

func (a *adam) step(model, grad *Model, lr float64, scale float32) {
	const b1, b2, eps, clip, wd = 0.9, 0.999, 1e-8, 1.0, 0.01
	var norm float64
	for _, p := range grad.params() {
		for _, v := range p {
			norm += float64(v*scale) * float64(v*scale)
		}
	}
	if norm = math.Sqrt(norm); norm > clip {
		scale *= float32(clip / norm)
	}
	a.t++
	c1 := 1 - math.Pow(b1, float64(a.t))
	c2 := 1 - math.Pow(b2, float64(a.t))
	gp := grad.params()
	for pi, p := range model.params() {
		mm, vv := a.m[pi], a.v[pi]
		decay := len(p) > model.C*4 // weight decay for matrices only
		for i := range p {
			g := gp[pi][i] * scale
			mm[i] = b1*mm[i] + (1-b1)*g
			vv[i] = b2*vv[i] + (1-b2)*g*g
			upd := (float64(mm[i]) / c1) / (math.Sqrt(float64(vv[i])/c2) + eps)
			if decay {
				upd += wd * float64(p[i])
			}
			p[i] -= float32(lr * upd)
		}
	}
}
