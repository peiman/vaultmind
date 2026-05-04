---
id: concept-perceptron
type: concept
title: Perceptron
created: 2026-04-29
aliases:
  - Rosenblatt Perceptron
  - Single-Layer Perceptron
tags:
  - deep-learning
  - machine-learning
  - history
  - linear-classifier
related_ids:
  - concept-multilayer-perceptron
  - concept-activation-function
  - concept-gradient-descent
  - concept-hebbian-learning
source_ids:
  - source-rosenblatt-1958
  - source-minsky-papert-1969
  - source-wikipedia-perceptron
---

## Overview

The perceptron is the earliest trainable artificial neuron, introduced by Frank Rosenblatt in 1958 at the Cornell Aeronautical Laboratory. It is a binary linear classifier that maps an input vector x to an output y in {0, 1} by computing a weighted sum of inputs and passing it through a step (Heaviside) activation: y = 1 if w·x + b > 0, else 0. Rosenblatt's design was directly inspired by McCulloch and Pitts' 1943 model of the biological neuron and by Hebb's 1949 learning rule.

The perceptron occupies a load-bearing place in the history of machine learning. It was the first algorithm shown to *learn* its own weights from labeled examples, and it framed pattern recognition as an optimization problem decades before that framing became standard. The 1969 critique by Minsky and Papert — showing that single-layer perceptrons cannot represent the XOR function or any non-linearly-separable problem — contributed to the first "AI winter" and stalled neural-network research until the rediscovery of [[backpropagation|backpropagation]] in the 1980s.

## How It Works

The perceptron learning rule is an online, error-driven update. For each training example (x, t) where t is the target label:

1. Compute prediction: y = step(w·x + b)
2. Update weights: w ← w + η (t − y) x
3. Update bias: b ← b + η (t − y)

Here η is the learning rate. If the prediction matches the target, no update occurs. If the data is linearly separable, Rosenblatt's perceptron convergence theorem guarantees that this rule finds a separating hyperplane in a finite number of steps.

## Key Properties

- **Linear decision boundary:** The set {x : w·x + b = 0} is a hyperplane. The perceptron can only solve linearly separable problems.
- **Online learning:** Weights are updated one example at a time — a precursor to [[stochastic-gradient-descent|stochastic gradient descent]].
- **No probabilistic output:** The step activation gives a hard classification, not a probability. Logistic regression, which replaces the step with a sigmoid, is the differentiable cousin.
- **XOR limitation:** A single perceptron cannot learn XOR because XOR is not linearly separable. This limitation is removed by stacking layers — the [[multilayer-perceptron|multilayer perceptron]].

## Connections

The perceptron is the atomic unit from which deeper networks are constructed. Replace the step activation with a smooth [[activation-function|activation function]] (sigmoid, ReLU, tanh) and stack multiple layers, and the result is a [[multilayer-perceptron|multilayer perceptron]] trainable via [[backpropagation|backpropagation]]. The biological inspiration ties the perceptron to [[hebbian-learning|Hebbian learning]] — both encode the principle that connection strengths should adapt to input-output correlations, though Hebb's rule is unsupervised while Rosenblatt's is supervised.
