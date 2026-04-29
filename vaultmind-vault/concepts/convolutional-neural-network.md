---
id: concept-convolutional-neural-network
type: concept
title: Convolutional Neural Network
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - CNN
  - ConvNet
  - Convolutional Network
tags:
  - deep-learning
  - architectures
  - computer-vision
  - cnn
related_ids:
  - concept-vision-transformer
  - concept-perceptron
  - concept-multilayer-perceptron
  - concept-backpropagation
  - concept-activation-function
source_ids:
  - source-lecun-1998
  - source-krizhevsky-2012
---

## Overview

A convolutional neural network (CNN) is a feedforward neural architecture specialized for grid-structured inputs — most famously 2D images, but also 1D signals and 3D volumes. CNNs replace the dense matrix multiplications of an MLP with learned convolution kernels that are slid spatially across the input. The same kernel is reused at every position (weight sharing), which makes the layer translation equivariant: shifting the input shifts the output by the same amount.

The lineage runs from Fukushima's Neocognitron (1980), through LeCun et al.'s LeNet-5 (1998) for handwritten digits, to Krizhevsky, Sutskever & Hinton's AlexNet (2012), which won ImageNet by a wide margin and triggered the modern deep learning revolution. VGG (2014), GoogLeNet/Inception (2014), and ResNet (2015) deepened and refined the template; ResNet's residual connections enabled training networks with 100+ layers.

## How It Works

A standard CNN block alternates three operations:

- **Convolution layer:** Each filter is a small tensor (e.g., 3x3xC) convolved with the input feature map, producing one output channel. A layer learns many filters in parallel, each detecting a local pattern. Early layers capture edges and textures; later layers compose those into parts and objects.
- **Nonlinearity:** Typically ReLU (post-AlexNet); historically tanh or sigmoid. See [[activation-function|Activation Function]].
- **Pooling (subsampling):** Max-pooling or average-pooling shrinks the spatial extent and provides limited translation invariance. Strided convolutions are now often used in place of explicit pooling.

After several conv-pool blocks, the spatial map is flattened and passed through fully-connected layers ([[multilayer-perceptron|MLP]]) culminating in a softmax classifier. Training uses [[backpropagation|backpropagation]] end-to-end with cross-entropy loss.

## Key Properties

- **Weight sharing:** A single filter's parameters apply across all spatial positions, dramatically reducing parameter count vs. a fully-connected layer of the same input/output size.
- **Local connectivity:** Each unit sees only a small receptive field of the layer below; the effective receptive field grows with depth.
- **Translation equivariance:** Built into the convolution operator; pooling adds approximate translation invariance.
- **Hierarchical features:** Stacked layers produce a hierarchy from low-level (edges) to high-level (semantic objects), learned end-to-end.
- **Inductive bias matched to images:** The architecture encodes locality and translation symmetry as priors, making CNNs sample-efficient on visual data — they generalize from less data than a comparably-sized [[transformer|Transformer]] or [[vision-transformer|ViT]].

## Connections

CNNs were the dominant computer vision architecture from 2012 until the [[vision-transformer|Vision Transformer]] (Dosovitskiy et al. 2020) showed that [[self-attention|self-attention]] over image patches matches or exceeds CNN performance at scale. The two paradigms differ in their inductive biases: CNNs hard-code locality and translation equivariance; ViTs learn whatever spatial relations the data supports, which requires more data but allows more flexible global integration.

CNNs also sit at the foundation of dense embedding pipelines for vision retrieval — visual analogues of text [[embedding-based-retrieval|embedding-based retrieval]]. For VaultMind, CNNs are not directly load-bearing (the vault is text), but the architectural lessons — hierarchical composition, weight sharing as inductive bias, end-to-end training — recur across every modality.
