package brain

import "math"

// cache keeps the attention keys and values of every token seen so far (a KV cache), so
// generating the next letter only runs the new position through the network instead of
// recomputing the whole sequence. The result is the same as forward, just much faster.
type cache struct {
	k, v [][]float32 // per layer, T×C
	n    int

	x, ln, qkv, att, proj, fc, act, out []float32 // scratch for one position
	mean, rstd, scores, probs           []float32
}

func (m *Model) newCache() *cache {
	C, T := m.C, m.T
	f := func(n int) []float32 { return make([]float32, n) }
	c := &cache{
		x: f(C), ln: f(C), qkv: f(3 * C), att: f(C), proj: f(C), fc: f(4 * C), act: f(4 * C), out: f(C),
		mean: f(1), rstd: f(1), scores: f(T), probs: f(m.V()),
	}
	for range m.L {
		c.k = append(c.k, f(T*C))
		c.v = append(c.v, f(T*C))
	}
	return c
}

// push runs one token at the next position and returns the next-token probabilities.
// The returned slice is reused by the following call.
func (m *Model) push(c *cache, tok int) []float32 {
	C, NH := m.C, m.NH
	hs := C / NH
	scale := 1 / float32(math.Sqrt(float64(hs)))
	pos := c.n
	for i := range C {
		c.x[i] = m.wte[tok*C+i] + m.wpe[pos*C+i]
	}
	for li, l := range m.layers {
		layernorm(c.ln, c.mean, c.rstd, c.x, l.ln1w, l.ln1b, 1, C)
		matmul(c.qkv, c.ln, l.qkvw, l.qkvb, 1, C, 3*C)
		copy(c.k[li][pos*C:(pos+1)*C], c.qkv[C:2*C])
		copy(c.v[li][pos*C:(pos+1)*C], c.qkv[2*C:3*C])
		for h := range NH {
			q := c.qkv[h*hs : (h+1)*hs]
			mx := float32(-1e30)
			for t := 0; t <= pos; t++ {
				c.scores[t] = dot(q, c.k[li][t*C+h*hs:t*C+(h+1)*hs]) * scale
				mx = max(mx, c.scores[t])
			}
			var sum float32
			for t := 0; t <= pos; t++ {
				c.scores[t] = float32(math.Exp(float64(c.scores[t] - mx)))
				sum += c.scores[t]
			}
			o := c.att[h*hs : (h+1)*hs]
			clear(o)
			for t := 0; t <= pos; t++ {
				w := c.scores[t] / sum
				v := c.v[li][t*C+h*hs : t*C+(h+1)*hs]
				for i := range hs {
					o[i] += w * v[i]
				}
			}
		}
		matmul(c.proj, c.att, l.projw, l.projb, 1, C, C)
		for i := range C {
			c.x[i] += c.proj[i]
		}
		layernorm(c.ln, c.mean, c.rstd, c.x, l.ln2w, l.ln2b, 1, C)
		matmul(c.fc, c.ln, l.fcw, l.fcb, 1, C, 4*C)
		gelu(c.act, c.fc)
		matmul(c.out, c.act, l.fc2w, l.fc2b, 1, 4*C, C)
		for i := range C {
			c.x[i] += c.out[i]
		}
	}
	layernorm(c.ln, c.mean, c.rstd, c.x, m.lnfw, m.lnfb, 1, C)
	matmul(c.probs, c.ln, m.wte, nil, 1, C, m.V())
	mx := c.probs[0]
	for _, p := range c.probs {
		mx = max(mx, p)
	}
	var sum float32
	for i, p := range c.probs {
		c.probs[i] = float32(math.Exp(float64(p - mx)))
		sum += c.probs[i]
	}
	for i := range c.probs {
		c.probs[i] /= sum
	}
	c.n++
	return c.probs
}
