---
id: concept-stochastic-gradient-descent
type: concept
title: Stochastic Gradient Descent
created: 2026-04-29
aliases:
  - SGD
  - Mini-Batch Gradient Descent
  - SGD with Momentum
tags:
  - deep-learning
  - optimization
  - stochastic-optimization
related_ids:
  - concept-gradient-descent
  - concept-backpropagation
  - concept-loss-function
  - concept-multilayer-perceptron
source_ids:
  - source-robbins-monro-1951
  - source-kingma-ba-2014
  - source-bottou-2010
---

## Overview

Stochastic gradient descent (SGD) is a variant of [[gradient-descent|gradient descent]] that estimates the gradient of the training loss using a single example or a small mini-batch rather than the entire training set. Each step is noisier but vastly cheaper, and the noise itself acts as an implicit regularizer that helps escape sharp local minima and saddle points on non-convex deep-learning losses.

SGD is the de facto optimizer for training large neural networks. Its lineage goes back to Robbins and Monro's 1951 stochastic approximation method. Modern deep-learning practice uses SGD with momentum or one of its adaptive descendants — most prominently Adam (Kingma & Ba, 2014).

## How It Works

For a training loss that decomposes as f(θ) = (1/N) Σᵢ fᵢ(θ) over N examples, the SGD update at step t is:

θ_{t+1} = θ_t − η ∇f_{B_t}(θ_t)

where B_t is a randomly sampled mini-batch and ∇f_{B_t}(θ_t) is the average gradient over that batch. Common variants:

- **Mini-batch size:** Batch size 1 is true (online) SGD; sizes from 32 to several thousand are typical for deep learning. Larger batches give lower-variance gradient estimates but require proportionally more compute per step.
- **Momentum (Polyak, 1964):** v_{t+1} = β v_t + ∇f_{B_t}(θ_t); θ_{t+1} = θ_t − η v_{t+1}. Smooths the trajectory and accelerates progress along consistent gradient directions.
- **Nesterov momentum:** Looks ahead before computing the gradient.
- **Adam (Adaptive Moment Estimation):** Maintains exponential moving averages of the gradient (first moment) and squared gradient (second moment), and divides the update by the square root of the second moment. This rescales updates per-parameter, which often makes Adam work well out-of-the-box without learning-rate tuning. AdamW decouples weight decay from the gradient step.

## Key Properties

- **Cheap per step, more steps:** SGD makes O(N/batch_size) updates per epoch instead of one — the throughput advantage that lets it scale to massive datasets.
- **Noise as regularization:** Mini-batch gradient noise prevents convergence to overly sharp minima and is associated with better generalization.
- **Requires learning-rate tuning:** Vanilla SGD is sensitive to η. Schedules (warm-up, cosine, step decay) are standard. Adaptive methods like Adam reduce this sensitivity.
- **Generalization gap with large batches:** Empirically, very large batch sizes can hurt generalization unless the learning rate is scaled accordingly (linear scaling rule).

## Connections

SGD is the practical implementation of [[gradient-descent|gradient descent]] for [[multilayer-perceptron|neural network]] training. Each step still requires a [[backpropagation|backpropagation]] pass to compute the mini-batch gradient. The original [[perceptron|perceptron]] learning rule is a per-example stochastic update — historically the first instance of stochastic gradient-style learning in neural networks. SGD optimizes a [[loss-function|loss function]], whose choice shapes the gradient signal: cross-entropy + softmax produces well-scaled gradients; MSE on classification produces vanishing-gradient regions.
