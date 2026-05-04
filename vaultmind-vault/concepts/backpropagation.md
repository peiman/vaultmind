---
id: concept-backpropagation
type: concept
title: Backpropagation
created: 2026-04-29
aliases:
  - Backprop
  - Reverse-Mode Automatic Differentiation
  - Error Backpropagation
tags:
  - deep-learning
  - optimization
  - automatic-differentiation
related_ids:
  - concept-multilayer-perceptron
  - concept-gradient-descent
  - concept-stochastic-gradient-descent
  - concept-loss-function
  - concept-activation-function
source_ids:
  - source-rumelhart-1986
  - source-linnainmaa-1970
  - source-goodfellow-2016
---

## Overview

Backpropagation is an algorithm for computing the gradient of a scalar loss with respect to every parameter of a feedforward neural network in a single backward pass. It is a special case of reverse-mode automatic differentiation applied to a [[multilayer-perceptron|multilayer perceptron]] (or any computational graph). The algorithm was popularized for neural networks by Rumelhart, Hinton, and Williams in 1986, though its mathematical core — the chain rule applied to nested differentiable functions — had been described earlier (Linnainmaa's 1970 master's thesis is widely credited as the first explicit formulation of reverse-mode AD).

Backprop is what makes [[gradient-descent|gradient descent]] practical on deep networks. Without it, computing per-parameter gradients would require either symbolic differentiation (which explodes in size) or finite differences (which is O(P) forward passes for P parameters). Backprop computes all gradients in time proportional to a single forward pass.

## How It Works

For a network with loss L, parameters W^(ℓ), pre-activations z^(ℓ), and activations a^(ℓ):

**Forward pass:** compute and cache z^(ℓ) and a^(ℓ) for every layer.

**Backward pass:** propagate the gradient signal δ^(ℓ) = ∂L/∂z^(ℓ) from output to input via the chain rule:

- Output layer: δ^(L) = ∇_a L ⊙ σ′(z^(L))
- Hidden layer: δ^(ℓ) = (W^(ℓ+1)ᵀ δ^(ℓ+1)) ⊙ σ′(z^(ℓ))
- Parameter gradients: ∂L/∂W^(ℓ) = δ^(ℓ) (a^(ℓ−1))ᵀ; ∂L/∂b^(ℓ) = δ^(ℓ)

The cached forward activations are exactly what's needed for the backward pass — this is why backprop's memory cost is proportional to network depth.

## Key Properties

- **Computational graph view:** Modern frameworks (PyTorch, JAX, TensorFlow) build a directed acyclic graph of operations during the forward pass, then walk it in reverse to compute gradients. Backprop generalizes naturally beyond MLPs to RNNs (backprop through time), CNNs, transformers, and arbitrary differentiable programs.
- **Requires differentiability:** Every operation in the forward pass must have a defined (sub-)gradient. This is why ReLU, despite a non-differentiable point at zero, works in practice — a sub-gradient of 0 or 1 is chosen.
- **Vanishing and exploding gradients:** When δ^(ℓ) is multiplied through many layers, its magnitude can decay to zero (vanishing) or grow without bound (exploding). This problem motivates ReLU activations, residual connections, batch normalization, and careful initialization.
- **Memory-time tradeoff:** Storing all forward activations costs memory; gradient checkpointing recomputes some activations on the backward pass to trade compute for memory.

## Connections

Backpropagation is the partner of [[gradient-descent|gradient descent]]: backprop produces the gradient, gradient descent uses it. Together they are the engine of [[multilayer-perceptron|MLP]] and modern deep-network training. Backprop's efficiency depends on differentiable [[activation-function|activation functions]] and a differentiable [[loss-function|loss function]]; replacing either with a non-differentiable surrogate breaks the algorithm. In practice, backprop is paired with [[stochastic-gradient-descent|stochastic gradient descent]] (or Adam) which uses mini-batch gradient estimates rather than full-dataset gradients.
