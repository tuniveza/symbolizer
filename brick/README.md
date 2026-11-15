<p align="center">
  <img src="docs/banner.svg" alt="brick: plain English in, a real Linux program out" width="100%">
</p>

<p align="center">
  <img alt="Linux x86-64" src="https://img.shields.io/badge/makes-Linux%20x86--64%20programs-e2402a?style=flat-square">
  <img alt="Python 3, standard library only" src="https://img.shields.io/badge/python-3%20·%20standard%20library%20only-ff8a3d?style=flat-square">
  <img alt="No C compiler, no libc" src="https://img.shields.io/badge/no%20C%20compiler-no%20libc-ffb15c?style=flat-square">
</p>

<h3 align="center">Write it in English. Run it on Linux.</h3>

<p align="center">
  brick turns everyday English into a real program:<br>
  a few kilobytes of machine code, every byte written by brick itself.
</p>

<p align="center">
  <a href="#-try-it">Try it</a> ·
  <a href="#-the-language">The language</a> ·
  <a href="#-brick-mode">Brick mode</a> ·
  <a href="#-how-it-works">How it works</a> ·
  <a href="#-it-learns">It learns</a> ·
  <a href="#-commands">Commands</a>
</p>

<br>

## ✨ Try it

`game.txt`:

```text
write get ready, wait a random number from 1 to 5 seconds, write GO in green,
start a timer as t, wait for a key, write you took t seconds
```

```console
$ brick game.txt
```

That's a reaction-time game, in one sentence.
`./game` is a few hundred bytes to a few KB: only the routines it uses go in. No Python inside, no C compiler, no libraries.

<br>

## 🚀 Get it

brick lives in [Symbolizer](https://github.com/tuniveza/symbolizer)'s `brick/` folder.
It needs **Linux on x86-64** and **Python 3**. Nothing to install.

```console
$ git clone https://github.com/tuniveza/symbolizer.git
$ cd symbolizer/brick
$ python3 brick.py texts/examples/hello.txt
```

`brick help` lists everything it understands.

<br>

## 📖 The language

One direction or many per line. Join them with commas, `then` or `and`.

```text
write hello there in bold red
set x to 7, write x divided by 2                       → 3.50
ask "What is your name?" as name
if score is more than 10, write well done, otherwise write keep going
repeat 3 times: write hello
for each name in names write hello name
save score to best.txt
wait 500 milliseconds
put the console on the left side of the screen
```

**Windows of its own.** The program speaks Wayland and paints every pixel itself:

```text
open a window called Painter, 500 by 360, in navy
make a button called go saying Go at 20 20, 120 by 40, in orange with round corners
make a canvas called board below go, 400 by 240
when board is clicked, draw a red dot on board at the mouse x and the mouse y
```

Short is fine too: `write red text on window at left side` writes "red text" in red at the window's left.

**Your own words:**

```text
zap means write
to greet someone means write hi someone in green
```

**Messy is fine.** This builds the same program as the tidy version:

```text
ok so TOP of screen hello there in red;bg blue   then 2 secs wait lol
```

Finished programs to read and run are in [`texts/examples`](texts/examples).

<br>

## 🧱 Brick mode

`sy brick` opens brick inside Symbolizer.
Every word is coloured by what brick reads it as. Under it, a panel shows brick's whole system.

| Key | |
|:---:|:---|
| **Alt + R** | run it: the brain checks it first and writes in its fix |
| **Alt + C** | check it, and fix missing or misread steps |
| **Alt + P** | keys to the panel and back |
| **Alt + N** | next panel: Bricks · Words · Decisions · Imagination · Neurons · Learning |
| **AltGr + D** | learning from your diary on / off |
| **AltGr + V** | the brains' voices on / off |
| **F12** | learning on / off, in any Symbolizer mode |

The **Bricks** panel shows every brick in use, live as you type, and what the brain thinks of each line.

<br>

## ⚙️ How it works

No C, no Python in between. Straight from words to machine code.

```text
English  →  read  →  split into actions  →  x86-64 code  →  ELF file  →  ./game
```

Unsure of a word? The dictionary, WordNet, ConceptNet and the neurons help.
Every guess is built and checked before it's used.
When brick guessed, it tells you how it read the line.

<br>

## 🧠 It learns

brick keeps learning from what you write, by itself.

- **Imagination neurons** predict the next word and propose new links. The compiler approves or rejects each one.
- **New bricks** are made from what it finds, only when they build exactly right.
- **A daemon** keeps learning in the background, at the lowest priority.
- **The language model teaches the brain.** What it reads right in your lines becomes a lesson: a reading, or a
  pattern with gaps (`bg a colour = set the background colour to a colour`), kept only when it builds exactly right.
- **New actions**: a word brick has no action for gets one made of what brick can already do
  (`to shout something means write something in bold then beep`), proved by building it.
- **The archetypes write practice lines** in their own voices, each with a known meaning, so brick tests itself.
- **Voices** (off by default): each archetype says out loud what it's learning, in its own voice.
- **Everything stays on your computer.** Only single words go to a public dictionary.

All it learns is plain text in `trainer/data/`, to read, change or delete.

<br>

## 📟 Commands

| | |
|:---|:---|
| `brick game.txt` | build and run |
| `brick build game.txt` | build only |
| `brick gui` | open Brick mode in Symbolizer |
| `brick teach zap means write` | a new word, for every program |
| `brick export game.txt` | everything brick made, as plain text |
| `brick trainer neurons learn` | learn now (`score`, `repair FILE`, `words 20`) |
| `brick trainer neurons always` | keep learning in the background (`off`, `status`) |
| `brick trainer lessons 20` | the language model's readings, taught to the brain |
| `brick trainer voices 10` | 10 practice lines in the archetypes' voices |
| `brick trainer narrator on` | the brains' voices (`off`, `status`) |
| `brick help` | every phrase it knows |

<br>

## ⚠️ Good to know

- Programs run on **64-bit Linux (x86-64)** only.
- **Moving the console** needs the Hyprland desktop. Elsewhere it's skipped.
- The optional **neural reader** needs Ollama and a graphics card.

<br>

## 🙏 Credits

Two open sources of English, both in `trainer/data/sources`:

- **WordNet 3.0** © 2006 Princeton University. All rights reserved. Used under the WordNet licence, included at the top of every file in [`trainer/data/sources/wordnet`](trainer/data/sources/wordnet). https://wordnet.princeton.edu
- **ConceptNet 5.7**, Commonsense Computing Initiative, https://conceptnet.io, under [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/). [`conceptnet.tsv`](trainer/data/sources/conceptnet.tsv) is a filtered part (links touching brick's words), made by [`trainer/conceptnet.py`](trainer/conceptnet.py), shared under the same licence.

The optional neural reader uses [Ollama](https://ollama.com) and [Qwen 2.5](https://github.com/QwenLM/Qwen2.5), not included.

<br>

<p align="center">
  <sub>Made by <a href="https://github.com/tuniveza">tuniveza</a>. Fired in the kiln, one brick at a time. 🧱🔥</sub>
</p>
