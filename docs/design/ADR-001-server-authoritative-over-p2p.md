# ADR-001: Server-authoritative over P2P

## Status

Accepted

## Context

Knowoff is a social deduction game whose core tension depends on hidden information: one or more players (Donowers) do not see the round's Nown, and nobody knows who is a Donower until votes reveal roles. The game also has a currency economy (Noin) and matchmaking.

We considered two architectural models:

1. **Peer-to-peer (P2P)** with a lightweight signaling server. Clients exchange state directly.
2. **Server-authoritative**: a trusted central server owns role assignment, the phase clock, blind-window resolution, and the currency ledger.

## Decision

We will use a server-authoritative architecture.

## Consequences

- **Role secrecy can be enforced at a single choke point.** The server renders per-recipient payloads so Nown never reaches a Donower's device.
- **Clock and phase transitions are authoritative.** Turn deadlines, ballots, and result windows are resolved server-side; clients are display-only.
- **The currency ledger lives on the server.** Noin grants, spends, and conversions are durable and tamper-resistant.
- **Operational cost is higher than pure P2P**, but acceptable for a commercial online game where trust is the product.
- **Horizontal scaling is simpler** because room→node affinity removes the need for distributed game state; live matches stay in ephemeral memory on one node.
