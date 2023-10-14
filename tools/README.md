# migrate

To install dependencies:

```bash
bun install
```

To run:

```bash
bun run index.ts
```

Early versions of xlx-activity-go contained individual
database records for Tx and Rx, recording time in a single
`ts` column. Starting with the version of 20231012, each
module activity is represented by a single row with
`ts` and `tsoff` values.  `migrate` does a simple
conversion to the new format.

Rename `env.sample` to `.env`, and edit to point to your
database file.

