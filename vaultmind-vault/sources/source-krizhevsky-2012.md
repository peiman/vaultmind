---
id: source-krizhevsky-2012
type: source
title: "Krizhevsky, Sutskever & Hinton. ImageNet Classification with Deep Convolutional Neural Networks (2012)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://doi.org/10.1145/3065386"
aliases:
  - AlexNet paper
  - ImageNet 2012
tags:
  - deep-learning
  - architectures
  - cnn
  - computer-vision
related_ids:
  - concept-convolutional-neural-network
---

# Krizhevsky, Sutskever & Hinton — AlexNet (2012)

The paper that re-ignited deep learning. Originally NIPS 2012; reprinted in *Communications of the ACM* 60(6):84-90 (2017). AlexNet won the 2012 ImageNet Large Scale Visual Recognition Challenge with a top-5 error of 15.3% — over 10 percentage points better than the runner-up — using a convolutional neural network with 60M parameters trained on two GPUs.

Key engineering contributions that became standard practice: ReLU activations, dropout regularization, data augmentation, GPU-parallel training, local response normalization, and overlapping max-pooling. AlexNet's win triggered the modern deep learning era and is the canonical demonstration that large [[convolutional-neural-network|CNNs]] dominate image classification given enough data and compute.
