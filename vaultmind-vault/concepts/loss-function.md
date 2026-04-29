---
id: concept-loss-function
type: concept
title: Loss Function
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - Cost Function
  - Objective Function
  - Error Function
tags:
  - deep-learning
  - optimization
  - training
related_ids:
  - concept-gradient-descent
  - concept-stochastic-gradient-descent
  - concept-backpropagation
  - concept-multilayer-perceptron
  - concept-activation-function
source_ids:
  - source-goodfellow-2016
  - source-bishop-2006
  - source-wikipedia-loss-function
---

## Overview

A loss function L(ŷ, y) is a scalar measure of how far a model's prediction ŷ is from the target y. Training a neural network means finding parameters θ that minimize the expected loss over the data distribution, typically estimated as the average loss over a training set. The choice of loss function determines what "good" means for the model and shapes the gradients that [[backpropagation|backpropagation]] propagates through the network.

Most losses used in deep learning derive from a probabilistic interpretation: the loss is the negative log-likelihood of the data under a model-defined distribution. This is why squared error pairs with Gaussian-output regression and cross-entropy pairs with categorical-output classification — each is the NLL of the corresponding distribution.

## Key Properties

- **Differentiable (almost everywhere):** Required for [[gradient-descent|gradient-descent]]-based training. Hinge loss and L1 loss have non-differentiable points but well-defined sub-gradients.
- **Decomposable:** Most losses are an average over per-example losses, which is what enables [[stochastic-gradient-descent|mini-batch SGD]].
- **Calibrated to output activation:** The output [[activation-function|activation]] and the loss must match for stable training. Softmax + cross-entropy and sigmoid + binary cross-entropy are standard pairings; using MSE on top of a softmax produces vanishingly small gradients in confident regions.
- **Surrogate vs. true objective:** Cross-entropy is a surrogate for the 0-1 classification error we actually care about — chosen because it is differentiable and convex in the logits, while 0-1 error is neither.

## How It Works — Common Losses

**Mean squared error (MSE):** L(ŷ, y) = (1/n) Σᵢ (ŷᵢ − yᵢ)². Standard for regression. Equivalent to negative log-likelihood under a Gaussian noise assumption with fixed variance. Penalizes large errors quadratically — sensitive to outliers.

**Mean absolute error (L1):** L(ŷ, y) = (1/n) Σᵢ |ŷᵢ − yᵢ|. Robust to outliers but non-differentiable at zero.

**Binary cross-entropy:** L(ŷ, y) = −[y log ŷ + (1 − y) log(1 − ŷ)]. Standard for binary classification with a sigmoid output.

**Categorical cross-entropy:** L(ŷ, y) = −Σ_k yₖ log ŷₖ. Standard for K-class classification with a softmax output. Equivalent to NLL of a categorical distribution. Combined with softmax, the gradient of the logits simplifies to ŷ − y, which is well-conditioned regardless of how confident the prediction is.

**Hinge loss:** L(ŷ, y) = max(0, 1 − y·ŷ). The classical SVM loss; encourages a margin of at least 1 between classes.

**Contrastive / triplet losses:** Used in metric learning and dense retrieval; pull together positive pairs and push apart negatives in embedding space.

## Connections

The loss function is the objective that [[gradient-descent|gradient descent]] (and its stochastic variants) minimize. Every parameter update in a [[multilayer-perceptron|neural network]] traces back to ∂L/∂θ computed by [[backpropagation|backpropagation]]. The choice of loss is tightly coupled to the output [[activation-function|activation]] — softmax-with-cross-entropy is the canonical pairing for classification because their gradients compose cleanly. In retrieval contexts, contrastive losses are the bridge from MLP-style supervised learning to [[dense-passage-retrieval|dense passage retrieval]] and [[embedding-based-retrieval|embedding-based retrieval]], where the network learns to map similar items to nearby vectors.
