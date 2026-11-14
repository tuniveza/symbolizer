"""brick's trainer: everything brick learns, in one place.

The trainer loop (core.py, once called the trainer) practises by itself while brick runs. Around it:
the brain (trainer/brain.py: words linked by how they're used), the textual universe (trainer/universe.py, with
ConceptNet, WordNet and synchronicity), a universe per symbol (trainer/symbols.py), the senses, sight and eye
(trainer/senses.py, trainer/sight.py, trainer/eye.py), the imagination (trainer/imagine.py), the neural reader (trainer/neural.py), practice on
scrambled directions (trainer/scrambler.py), and what a person decides (trainer/review.py, trainer/outliers.py).
What it has learned lives in trainer/data. Commands: brick trainer help.
"""
