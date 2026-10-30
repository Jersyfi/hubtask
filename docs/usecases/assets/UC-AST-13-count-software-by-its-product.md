---
id: UC-AST-13
title: Count software by its product
context: assets
actors: [PE-admin, PE-member, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18]
state: partial
tasks: []
checked_by: [core/domain/model/cmdb/Assurance_test.go, core/application/service/cmdb/Assurance_test.go, test/integration/cmdb_assurance_test.go]
---

# Count software by its product

## Goal

However installations spell what they run, the inventory counts it as one product with its versions and how many hosts have each, and an administrator names the spellings folding does not catch.

## Story

The scans report "Mozilla Firefox 128.0 (x64)" and "mozilla firefox", and Code from Microsoft Corporation. The software view shows mozilla firefox with two versions and Code as nothing anybody recognises - until Dana adds the alias "code" for Visual Studio Code by Microsoft.

## How to check

1. A hub's software CIs are grouped by product: the publisher folded without case and legal form ("Microsoft Corporation" is "microsoft"), the product without case, version or architecture ("Firefox 128.0 (x64)" is "firefox"); each product lists its versions with how many CIs and installations (hosts it is installed on) each.
2. An administrator defines an alias - a product and a publisher as reported, or any publisher - and the publisher and product it counts as; an alias with its publisher wins over one for any, and removing it changes no CI.
3. The *Assets* screen shows a hub's software by product and the workspace's aliases, where an administrator adds and removes one.

## Where it ends

* No licence compliance position (research A4), no vulnerability matching (A7), no shipped catalogue of products.

## Today

* Check 3: not met — the web app has no software view yet.
