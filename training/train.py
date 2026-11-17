"""Train Mansur's brain on the Mac GPU (PyTorch MPS) and export it in the format brain/io.go reads.

The model mirrors brain/gpt.go exactly: learned token and position embeddings, pre-norm blocks with
LayerNorm (eps 1e-5), fused QKV with biases, causal attention, a GELU (tanh) MLP of width 4C, a final
LayerNorm and an output layer tied to the token embeddings. Training data comes from

    go run ./cmd/train -dump training/data

so tokenization is identical to the Go code. Dropout is used only during training, so the exported
model is unchanged. Whenever the validation loss improves the script writes training/out/weights.bin,
so the best epoch is kept and an interrupted run loses at most one epoch.

    training/.venv/bin/python training/train.py --epochs 30
"""

import argparse
import json
import math
import random
import struct
import time
from pathlib import Path

import numpy as np
import torch
import torch.nn as nn
import torch.nn.functional as F

ROOT = Path(__file__).resolve().parent


class Block(nn.Module):
    def __init__(self, C, NH, dropout=0.0):
        super().__init__()
        self.NH = NH
        self.drop = nn.Dropout(dropout)
        self.ln1 = nn.LayerNorm(C, eps=1e-5)
        self.qkv = nn.Linear(C, 3 * C)
        self.proj = nn.Linear(C, C)
        self.ln2 = nn.LayerNorm(C, eps=1e-5)
        self.fc = nn.Linear(C, 4 * C)
        self.fc2 = nn.Linear(4 * C, C)

    def forward(self, x):
        B, T, C = x.shape
        q, k, v = self.qkv(self.ln1(x)).split(C, dim=2)
        heads = lambda t: t.view(B, T, self.NH, C // self.NH).transpose(1, 2)
        a = F.scaled_dot_product_attention(heads(q), heads(k), heads(v), is_causal=True)
        x = x + self.drop(self.proj(a.transpose(1, 2).reshape(B, T, C)))
        return x + self.drop(self.fc2(F.gelu(self.fc(self.ln2(x)), approximate="tanh")))


class GPT(nn.Module):
    def __init__(self, V, C, L, NH, T, dropout=0.0):
        super().__init__()
        self.wte = nn.Embedding(V, C)
        self.wpe = nn.Embedding(T, C)
        self.drop = nn.Dropout(dropout)
        self.blocks = nn.ModuleList(Block(C, NH, dropout) for _ in range(L))
        self.lnf = nn.LayerNorm(C, eps=1e-5)
        for name, p in self.named_parameters():
            if name.endswith("bias"):
                nn.init.zeros_(p)
            elif ".ln" in name or name.startswith("lnf"):
                nn.init.ones_(p)
            else:
                nn.init.normal_(p, std=0.02)

    def forward(self, idx):
        x = self.drop(self.wte(idx) + self.wpe(torch.arange(idx.shape[1], device=idx.device)))
        for b in self.blocks:
            x = b(x)
        return self.lnf(x) @ self.wte.weight.T

    def tensors(self):
        """Parameters in the exact order of Model.params() in brain/gpt.go."""
        out = [self.wte.weight, self.wpe.weight]
        for b in self.blocks:
            out += [b.ln1.weight, b.ln1.bias, b.qkv.weight, b.qkv.bias, b.proj.weight, b.proj.bias,
                    b.ln2.weight, b.ln2.bias, b.fc.weight, b.fc.bias, b.fc2.weight, b.fc2.bias]
        return out + [self.lnf.weight, self.lnf.bias]


GROUP = 64


def quantize(flat):
    """int8 with one float32 scale per group of 64, rounding half away from zero like Go's math.Round."""
    out = []
    for g in range(0, len(flat), GROUP):
        chunk = flat[g:g + GROUP].astype(np.float32)
        scale = np.float32(np.abs(chunk).max() / np.float32(127))
        if scale == 0:
            scale = np.float32(1)
        q = np.sign(chunk / scale) * np.floor(np.abs(chunk / scale) + 0.5)
        out.append((scale, q.astype(np.int8)))
    return out


def export(model, meta, path):
    buf = bytearray(b"PUFI")
    moods = meta["moods"]
    buf += struct.pack("<5i", meta["C"], meta["L"], meta["NH"], meta["T"], len(moods))
    for m in moods:
        b = m.encode()
        buf += bytes([len(b)]) + b
    chars = meta["chars"]
    buf += struct.pack("<i", len(chars)) + struct.pack(f"<{len(chars)}i", *map(ord, chars))
    for t in model.tensors():
        for scale, q in quantize(t.detach().float().cpu().numpy().ravel()):
            buf += struct.pack("<f", scale) + q.tobytes()
    path.write_bytes(buf)


def dequantized(model, path):
    """A CPU copy of the model with the exact int8 weights from the exported file."""
    data = path.read_bytes()
    clone = GPT(model.wte.num_embeddings, model.wte.embedding_dim, len(model.blocks), model.blocks[0].NH,
                model.wpe.num_embeddings)
    off = 4
    C, L, NH, T, M = struct.unpack_from("<5i", data, off)
    off += 20
    for _ in range(M):
        off += 1 + data[off]
    (nc,) = struct.unpack_from("<i", data, off)
    off += 4 + 4 * nc
    with torch.no_grad():
        for t in clone.tensors():
            flat = np.empty(t.numel(), dtype=np.float32)
            for g in range(0, t.numel(), GROUP):
                n = min(GROUP, t.numel() - g)
                (scale,) = struct.unpack_from("<f", data, off)
                q = np.frombuffer(data, dtype=np.int8, count=n, offset=off + 4)
                flat[g:g + n] = q.astype(np.float32) * scale
                off += 4 + n
            t.copy_(torch.from_numpy(flat).view_as(t))
    return clone


def load(data_dir):
    meta = json.loads((data_dir / "meta.json").read_text())
    seqs = []
    for line in (data_dir / "samples.txt").read_text().splitlines():
        nums = list(map(int, line.split()))
        seqs.append((nums[0], nums[1:]))
    return meta, seqs


def batches(seqs, size, shuffle):
    order = list(range(len(seqs)))
    if shuffle:
        random.shuffle(order)
    # Bucket by length so padding stays small.
    chunks = [order[i:i + size * 50] for i in range(0, len(order), size * 50)]
    for chunk in chunks:
        chunk.sort(key=lambda i: len(seqs[i][1]))
        group = [chunk[i:i + size] for i in range(0, len(chunk), size)]
        if shuffle:
            random.shuffle(group)
        yield from group


def tensorize(seqs, ids, device):
    n = max(len(seqs[i][1]) for i in ids) - 1
    x = torch.zeros(len(ids), n, dtype=torch.long)
    y = torch.full((len(ids), n), -100, dtype=torch.long)  # -100 is ignored by the loss
    for row, i in enumerate(ids):
        start, toks = seqs[i]
        t = torch.tensor(toks)
        x[row, :len(toks) - 1] = t[:-1]
        y[row, start:len(toks) - 1] = t[start + 1:]
    return x.to(device), y.to(device)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--data", default=ROOT / "data", type=Path)
    ap.add_argument("--out", default=ROOT / "out", type=Path)
    ap.add_argument("--epochs", default=30, type=int)
    ap.add_argument("--batch", default=64, type=int)
    ap.add_argument("--lr", default=3e-3, type=float)
    ap.add_argument("--seed", default=1, type=int)
    ap.add_argument("--dropout", default=0.1, type=float)
    args = ap.parse_args()

    random.seed(args.seed)
    torch.manual_seed(args.seed)
    device = "mps" if torch.backends.mps.is_available() else "cpu"
    meta, seqs = load(args.data)
    random.shuffle(seqs)
    val, train = seqs[:len(seqs) // 50], seqs[len(seqs) // 50:]
    model = GPT(meta["vocab"], meta["C"], meta["L"], meta["NH"], meta["T"], args.dropout).to(device)
    params = sum(p.numel() for p in model.parameters())
    print(f"{device}: {len(train)} train / {len(val)} val samples, {params} parameters", flush=True)

    C = meta["C"]
    decay = [p for p in model.parameters() if p.numel() > 4 * C]  # weight decay for matrices only, as in Go
    rest = [p for p in model.parameters() if p.numel() <= 4 * C]
    opt = torch.optim.AdamW([{"params": decay, "weight_decay": 0.01}, {"params": rest, "weight_decay": 0}],
                            lr=args.lr, betas=(0.9, 0.999), eps=1e-8)
    steps = args.epochs * math.ceil(len(train) / args.batch)
    sched = torch.optim.lr_scheduler.LambdaLR(
        opt, lambda s: min(1, (s + 1) / 100) * (0.1 + 0.9 * 0.5 * (1 + math.cos(math.pi * s / steps))))

    args.out.mkdir(parents=True, exist_ok=True)
    start = time.time()
    best = float("inf")
    for ep in range(1, args.epochs + 1):
        model.train()
        total, count = 0.0, 0
        for ids in batches(train, args.batch, True):
            x, y = tensorize(train, ids, device)
            loss = F.cross_entropy(model(x).flatten(0, 1), y.flatten(), ignore_index=-100)
            opt.zero_grad(set_to_none=True)
            loss.backward()
            torch.nn.utils.clip_grad_norm_(model.parameters(), 1.0)
            opt.step()
            sched.step()
            n = (y != -100).sum().item()
            total, count = total + loss.item() * n, count + n
        model.eval()
        vt, vc = 0.0, 0
        with torch.no_grad():
            for ids in batches(val, args.batch, False):
                x, y = tensorize(val, ids, device)
                vt += F.cross_entropy(model(x).flatten(0, 1), y.flatten(), ignore_index=-100, reduction="sum").item()
                vc += (y != -100).sum().item()
        mark = ""
        if vt / vc < best:
            best, mark = vt / vc, "  * saved"
            export(model, meta, args.out / "weights.bin")
            torch.save(model.state_dict(), args.out / "model.pt")
        print(f"epoch {ep:3d}  train {total / count:.3f}  val {vt / vc:.3f}  {time.time() - start:6.0f}s{mark}", flush=True)

    # Parity data for brain/parity_test.go: next-token probabilities of the exported int8 model.
    model.load_state_dict(torch.load(args.out / "model.pt"))
    clone = dequantized(model.cpu(), args.out / "weights.bin").eval()
    cases = []
    for _, toks in val[:8]:
        with torch.no_grad():
            probs = F.softmax(clone(torch.tensor([toks[:-1]])), dim=-1)[0, -1]
        cases.append({"toks": toks[:-1], "probs": probs.tolist()})
    (args.out / "parity.json").write_text(json.dumps(cases))
    print(f"wrote {args.out / 'weights.bin'} ({(args.out / 'weights.bin').stat().st_size // 1024} KB) and parity.json")


if __name__ == "__main__":
    main()
