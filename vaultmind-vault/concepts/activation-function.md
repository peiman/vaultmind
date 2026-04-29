---
id: concept-activation-function
type: concept
title: Activation Function
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - Non-Linearity
  - Transfer Function
tags:
  - deep-learning
  - neural-networks
  - non-linearity
related_ids:
  - concept-perceptron
  - concept-multilayer-perceptron
  - concept-backpropagation
  - concept-loss-function
source_ids:
  - source-glorot-2011
  - source-hendrycks-2016
  - source-goodfellow-2016
---

## Overview

An activation function is a (typically) non-linear scalar function applied element-wise to a layer's pre-activation z = Wx + b in a neural network. Without a non-linear activation between layers, a stack of linear transformations collapses into a single linear transformation, and the [[multilayer-perceptron|multilayer perceptron]] loses its expressive power. Non-linearity is what lets neural networks approximate arbitrary continuous functions (universal approximation theorem).

The choice of activation has large practical effects: it shapes the gradient signal during [[backpropagation|backpropagation]], determines whether deep networks suffer from vanishing/exploding gradients, and constrains the output range. Modern deep networks overwhelmingly use ReLU or one of its smooth descendants (GELU, SiLU/Swish) in hidden layers; output layers use softmax, sigmoid, or identity depending on the task.

## How It Works — Common Activations

**Step (Heaviside):** Used in the original [[perceptron|perceptron]]. Non-differentiable, so incompatible with gradient-based training.

**Sigmoid (logistic):** σ(z) = 1 / (1 + e^(−z)). Maps R → (0, 1). Smooth, differentiable, historically popular for hidden layers and still used for binary-classification output. Its derivative σ′(z) = σ(z)(1 − σ(z)) peaks at 0.25 and approaches zero in the saturated regions, which causes the **vanishing gradient problem** in deep networks: gradients shrink by at least a factor of 4 per layer during backprop.

**Tanh:** tanh(z) = (e^z − e^(−z)) / (e^z + e^(−z)). Maps R → (−1, 1). Zero-centered (unlike sigmoid), so optimization tends to be better-behaved, but still saturates and vanishes for large |z|.

**ReLU (Rectified Linear Unit):** ReLU(z) = max(0, z). Introduced as the default for deep networks by Glorot, Bordes, and Bengio (2011) and a key ingredient in the AlexNet result (2012). Cheap to compute, does not saturate for positive inputs, and produces sparse activations. Suffers from "dying ReLU" — units that get pushed into the z < 0 region stop receiving gradient and never recover. Variants: Leaky ReLU, Parametric ReLU (PReLU), and ELU address this.

**GELU (Gaussian Error Linear Unit):** GELU(z) = z · Φ(z), where Φ is the standard Gaussian CDF. Smooth approximation of ReLU that gates the input by its own value. Used in BERT, GPT, and most modern transformers (Hendrycks & Gimpel, 2016).

**SiLU / Swish:** SiLU(z) = z · σ(z). Self-gated activation similar to GELU; used in many modern architectures.

**Softmax (output layer):** softmax(z)ᵢ = e^{zᵢ} / Σⱼ e^{zⱼ}. Maps a vector to a probability distribution; standard pairing with categorical cross-entropy [[loss-function|loss]].

## Key Properties

- **Non-linearity is the point:** Without it, depth adds no expressive power.
- **Differentiability matters:** [[backpropagation|Backpropagation]] requires (sub-)gradients. ReLU's non-differentiable point at zero is handled by convention.
- **Gradient regime determines trainability:** Sigmoid and tanh saturate and vanish; ReLU/GELU/SiLU keep gradients alive in the active region. This is why deep networks transitioned from sigmoid to ReLU around 2010-2012.
- **Output activation must match the loss:** Softmax + cross-entropy for classification; sigmoid + binary cross-entropy for binary; identity + MSE for regression.

## Connections

Activation functions are the non-linear glue that turns a stack of linear layers into a [[multilayer-perceptron|multilayer perceptron]]. Their derivatives appear directly in [[backpropagation|backpropagation]]'s δ-recursion, which is why saturating activations like sigmoid produce vanishing gradients in deep networks. The output activation is tightly coupled to the [[loss-function|loss function]] — together they determine whether the gradient signal reaching the final layer is well-conditioned. The original [[perceptron|perceptron]]'s step activation made gradient-based training impossible; replacing it with a smooth activation is precisely what unlocked deep learning.
