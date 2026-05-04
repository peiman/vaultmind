---
id: concept-gradient-descent
type: concept
title: Gradient Descent
created: 2026-04-29
aliases:
  - Steepest Descent
  - Batch Gradient Descent
tags:
  - deep-learning
  - optimization
  - first-order-method
related_ids:
  - concept-stochastic-gradient-descent
  - concept-backpropagation
  - concept-loss-function
  - concept-multilayer-perceptron
source_ids:
  - source-cauchy-1847
  - source-goodfellow-2016
  - source-wikipedia-gradient-descent
---

## Overview

Gradient descent is a first-order iterative optimization algorithm that minimizes a differentiable function f(θ) by repeatedly stepping in the direction of the negative gradient: θ ← θ − η ∇f(θ). The method dates to Cauchy (1847) and is the workhorse of modern machine learning, where f is a [[loss-function|loss function]] and θ are the parameters of a model such as a [[multilayer-perceptron|neural network]].

In the deep-learning setting, the gradient ∇f(θ) is computed by [[backpropagation|backpropagation]]. "Batch" or "full-batch" gradient descent uses the gradient of the loss over the entire training set per step; "mini-batch" and "stochastic" variants use noisier estimates from subsets, trading variance for throughput. See [[stochastic-gradient-descent|SGD]] for the practical variant used to train modern neural networks.

## How It Works

Given an objective f : R^n → R and a starting point θ_0, gradient descent iterates:

θ_{t+1} = θ_t − η_t ∇f(θ_t)

where η_t is the learning rate (step size) at iteration t. Choices that affect behavior:

- **Constant η:** Simple but sensitive — too large overshoots, too small crawls.
- **Learning-rate schedule:** Decay η over time (1/t, exponential, cosine) for convergence guarantees on convex problems.
- **Line search / adaptive step:** Choose η_t each step to satisfy Armijo or Wolfe conditions.

For a smooth, convex f with L-Lipschitz gradient, gradient descent with η ≤ 1/L converges to the global minimum at rate O(1/t). For strongly convex f, the rate is exponential. Neural-network losses are non-convex, so these guarantees do not apply — yet gradient descent (and its stochastic variants) still finds useful minima in practice.

## Key Properties

- **First-order:** Uses only the gradient, not the Hessian. Cheaper per step than Newton's method but has worse iteration complexity on ill-conditioned problems.
- **Local convergence:** Converges to a stationary point (∇f = 0), which on non-convex problems may be a local minimum, saddle point, or plateau rather than the global minimum.
- **Learning rate is critical:** Too high causes divergence; too low causes slow progress. Modern practice uses warm-up, decay, or adaptive methods (Adam, AdamW) to manage this.
- **Batch vs full-batch:** Full-batch gradient descent computes ∇f over the whole training set every step. For large datasets this is memory-prohibitive and usually wasteful — see [[stochastic-gradient-descent|SGD]] for the mini-batch variant that dominates practice.

## Connections

Gradient descent consumes the gradients produced by [[backpropagation|backpropagation]] and operates on the [[loss-function|loss function]] of a [[multilayer-perceptron|neural network]]. The mini-batch and momentum-augmented versions live under [[stochastic-gradient-descent|stochastic gradient descent]]. The original 1958 [[perceptron|perceptron]] learning rule can be viewed as stochastic (sub-)gradient descent on the perceptron criterion loss with a step-size of 1 — though Rosenblatt did not frame it that way at the time.
