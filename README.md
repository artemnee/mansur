<p align="center">
  <img src="docs/banner.png" alt="Mansur — a tiny GPT cat for macOS" width="760">
</p>

<p align="center">
  <b>A pixel-art alien cat that lives above your Dock and chats in English and Russian<br>
  through his own tiny GPT, written and trained from scratch.</b><br>
  Fully offline · no API, no account · the whole brain is 490 KB
</p>

<p align="center">
  <a href="https://github.com/artemnee/mansur/releases/latest"><img alt="macOS 11+" src="https://img.shields.io/badge/macOS-11%2B-9cc75a?style=flat-square&logo=apple&logoColor=white"></a>
  <a href="https://go.dev/dl/"><img alt="Go 1.27+" src="https://img.shields.io/badge/Go-1.27%2B-9cc75a?style=flat-square&logo=go&logoColor=white"></a>
  <a href="#the-brain"><img alt="Brain: 470K params" src="https://img.shields.io/badge/brain-470K%20params-9cc75a?style=flat-square"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-9cc75a?style=flat-square"></a>
</p>

<p align="center">
  <img src="docs/demo.gif" alt="Mansur says hi, walks, gets petted, jumps, eats a berry and falls asleep" width="416">
</p>

## Install

<p align="center">
  <a href="https://github.com/artemnee/mansur/releases/latest/download/Mansur.dmg"><img alt="Download for macOS" src="https://img.shields.io/badge/Download_for_macOS-.dmg-9cc75a?style=for-the-badge&logo=apple&logoColor=white"></a>
</p>

Or from the terminal:

```sh
curl -fsSL https://raw.githubusercontent.com/artemnee/mansur/main/install.sh | sh
```

Picks the build for your Mac, **verifies the checksum** and installs `mansur` to `~/.local/bin`. No `sudo`, no changes to your shell config. [Read the script](install.sh) first if you like.

<details>
<summary>Other ways: app, Go, manual download</summary>

**App:** open the downloaded `Mansur.dmg` and drag Mansur into Applications. The app is not signed with a paid Apple certificate, so on first launch open **System Settings → Privacy & Security → Open Anyway** once.

**Go 1.27+:** `go install github.com/artemnee/mansur@latest` (no Xcode, no cgo).

**Manual:** download `mansur-darwin-arm64.tar.gz` (Apple Silicon) or `mansur-darwin-amd64.tar.gz` (Intel) from Releases, check it with `shasum -a 256 -c SHA256SUMS`, unpack and move `mansur` into your `PATH`.

**Uninstall:** `mansur stop && rm ~/.local/bin/mansur` (his state lives in `~/Library/Application Support/mansur`).

</details>

**Updates:** install the same way again and restart him (`mansur stop && mansur`); his state is kept. If you turn on **updates** in his menu, he checks GitHub once a day and tells you when a new version is out. He never installs anything by himself.

## Use

```sh
mansur                 # move in (runs in the background)
mansur stop            # say goodbye
mansur status
mansur autostart on    # start at login
mansur -size 6         # a bigger cat
```

| You | Mansur |
|---|---|
| hover over him | purrs, hearts fly |
| click | jumps |
| drag | dangles, then lands on a window or the Dock |
| right click | menu: talk, feed, sleep, break reminders, autostart, language, updates |
| leave him be | strolls around his corner, follows the cursor, hops onto nearby windows, yawns, stretches, grooms, gets hungry |
| dart the cursor around him | hunts it and pounces |
| work 50 min straight | reminds you to take a break |
| tell him "my name is …" | remembers it and greets you by name |
| come back after a while | "you're back!", or "I missed you" after a few days |
| ignore him for hours | sulks, turns away and grumbles until you pet him |

**His character grows from how you treat him:** pet him a lot and he gets cuddly and comes to your cursor; feed him often and he gets greedy; talk to him and he gets chatty; leave him alone and he becomes independent. The menu shows what he has become.

<p align="center">
  <img src="docs/poses.png" alt="Idle, petted, jumping, eating, sleeping" width="640">
</p>

<table>
  <tr>
    <td align="center"><img src="docs/menu.png" alt="Pixel menu" width="300"><br><sub>His menu</sub></td>
    <td align="center"><img src="docs/chat.png" alt="Chatting with Mansur" width="420"><br><sub>He replies in the language you write in</sub></td>
  </tr>
</table>

## The brain

A decoder-only transformer (the GPT-2 design) written from scratch in Go. It runs inside the app on the CPU and answers in a fraction of a second.

| | |
|---|---|
| Size | 4 layers, 4 heads, width 96, 128-character context, ~470K parameters, **490 KB** in int8 |
| Languages | English and Russian; replies in the language of your message |
| Knows | ~120 small-talk topics, his own moods, simple facts, adding numbers up to 10, the time and date |
| Memory | the previous exchange: "what day is it?" → "and tomorrow?" |
| Honesty | says "I don't know" when unsure |
| Quality | 89% on 159 held-out questions, 100% in the right language ([`eval_test.go`](brain/eval_test.go)) |

The network writes templates like `{time_en}`, and the app fills in the real time, date and weekday, so those answers are always correct.

**Training data:** [`brain/dialogs.txt`](brain/dialogs.txt) (Russian) and [`brain/dialogs_en.txt`](brain/dialogs_en.txt) (English), expanded with paraphrases, typos and language switches by [`cmd/train`](cmd/train).

**Training** runs on the Mac GPU with PyTorch (~25 s per epoch). It is only needed to retrain; the app itself is pure Go.

```sh
go run ./cmd/train -dump training/data               # export tokenized samples
uv venv training/.venv && uv pip install --python training/.venv/bin/python -r training/requirements.txt
training/.venv/bin/python training/train.py           # writes training/out/weights.bin
cp training/out/weights.bin brain/ && go test -v ./brain # quality report
```

## Privacy

- **No network** and no telemetry. The one exception is the update check, and it is off until you turn it on in the menu: then once a day he asks GitHub for the latest version number ([`update.go`](update.go)). Nothing about you is sent.
- **No screen reading.** For climbing windows he reads only their positions, never contents or titles.
- **No keyboard or clipboard access** except in his own input box.
- His only files are `state.json` and a pid file, readable only by you. `state.json` holds his satiety and settings (including whether update checks are on), your name if you told him, the hours you are usually around and his character. Delete it and he starts fresh.

Releases are built by [GitHub Actions](.github/workflows/release.yml) straight from a tagged commit and ship with `SHA256SUMS`.

## Build

```sh
make build   # bin/mansur
make run     # run in the foreground
make dmg     # dist/Mansur.dmg (Apple Silicon + Intel)
MANSUR_ASSETS=docs go test -run TestReadmeAssets .   # re-render these images
```

## License

[MIT](LICENSE) © artemnee
