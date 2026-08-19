// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package markdown converts between Markdown and the format-agnostic
// [github.com/go-richdoc/richdoc] document model.
//
// [Parse] turns CommonMark source (with the GFM table and strikethrough
// extensions enabled) into a [richdoc.Document]. Parsing delegates to the
// reference Go Markdown library, goldmark, and walks its AST to build the
// richdoc tree, so the package never hand-rolls a CommonMark parser.
//
// [Write] renders a [richdoc.Document] back to clean CommonMark. The pair aims
// for a stable round-trip: Write(Parse(src)) is semantically equivalent to
// src, and Parse(Write(Parse(src))) reproduces the same richdoc tree, up to
// harmless normalisation (emphasis markers normalised to '*', list markers to
// '-'/'1.', and so on).
//
// The package is pure Go and builds with CGO disabled.
package markdown
