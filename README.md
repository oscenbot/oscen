# Oscen

Bot for interacting with Spotify, written in Go

## Components Overview

### `oscen`

The main binary. Listens on port 9000 for incoming HTTP connections and uses this to handle Spotify oauth callbacks and incoming events from discord.

### `oscen-cli`

A handy tool for engineers operating Oscen. 

### `oscen-presence`

A teeny-tiny-microservice that connects to the Discord websocket gateway. This is needed because the main binary does not connect to the gateway and this causes the bot to show as offline.

## Ops

ArgoCD is deployed into the cluster manually with Helm.