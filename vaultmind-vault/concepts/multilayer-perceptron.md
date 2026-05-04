---
id: concept-multilayer-perceptron
type: concept
title: Multilayer Perceptron
created: 2026-04-29
aliases:
  - MLP
  - Feedforward Neural Network
  - Fully Connected Network
tags:
  - deep-learning
  - neural-networks
  - feedforward
related_ids:
  - concept-perceptron
  - concept-backpropagation
  - concept-activation-function
  - concept-gradient-descent
  - concept-loss-function
source_ids:
  - source-hornik-1989
  - source-cybenko-1989
  - source-goodfellow-2016
---

## Overview

A multilayer perceptron (MLP) is a feedforward neural network composed of an input layer, one or more hidden layers, and an output layer, where each layer is fully connected to the next and uses a non-linear [[activation-function|activation function]]. Each unit computes a weighted sum of its inputs and applies a non-linearity, just like a [[perceptron|perceptron]] — but stacking such units in layers turns the model from a linear classifier into a universal function approximator.

MLPs are the canonical "deep learning" architecture for tabular and vector-valued data. Convolutional networks, transformers, and most other modern architectures contain MLP sub-blocks (e.g., the feedforward layer inside each transformer block). Training proceeds by [[backpropagation|backpropagation]] of error gradients through the layers, combined with a [[gradient-descent|gradient-descent]]-family optimizer.

## How It Works

For an L-layer MLP, the forward pass at layer ℓ is:

- z^(ℓ) = W^(ℓ) a^(ℓ−1) + b^(ℓ)
- a^(ℓ) = σ(z^(ℓ))

where σ is the activation, W^(ℓ) is the weight matrix, and b^(ℓ) is the bias vector. The first activation a^(0) is the input x, and the final activation a^(L) is the output prediction. Training minimizes a [[loss-function|loss function]] L(a^(L), y) by computing ∂L/∂W^(ℓ) and ∂L/∂b^(ℓ) for every layer via [[backpropagation|backpropagation]], then taking a gradient step.

## Key Properties

- **Universal approximation theorem:** Cybenko (1989) and Hornik et al. (1989) proved that an MLP with a single hidden layer of sufficient width and a non-constant, bounded, monotonically increasing activation can approximate any continuous function on a compact subset of R^n to arbitrary precision. Depth is not theoretically required, but in practice deeper networks generalize better with fewer parameters.
- **Non-linearity is essential:** Without a non-linear activation between layers, an MLP collapses to a single linear transformation regardless of depth.
- **Fully connected:** Every unit in layer ℓ receives input from every unit in layer ℓ−1. This makes MLPs parameter-heavy compared to convolutional or attention-based architectures that exploit structure.
- **Susceptible to vanishing/exploding gradients:** Deep MLPs with sigmoid or tanh activations suffer from gradients that shrink or blow up during backprop — one motivation for the [[activation-function|ReLU and GELU activations]].

## Connections

The MLP is what you get when you fix the [[perceptron|perceptron]]'s expressiveness problem by stacking layers and replacing the step function with a differentiable activation. The differentiability is what makes [[backpropagation|backpropagation]] possible, and backprop is what makes [[gradient-descent|gradient descent]] tractable on multi-layer architectures. The MLP also reappears as a sub-component inside more structured models — the FFN block in a transformer is a two-layer MLP applied position-wise.
