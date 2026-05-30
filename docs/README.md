# Documentation Index

This directory is the repository-local knowledge base for agentic work. The
root [AGENTS.md](../AGENTS.md) is a map; this directory is the system of record
for durable context, harness constraints, and deeper references.

## Agent Harness

- [Harness workflow](harness/README.md)
- [Golden rules](golden-rules/README.md)
- [Toolchain rule](golden-rules/toolchain.md)

## Bootstrap Key Reuse

- [A3 linear-transform schedule interning plan](bootstrap_key_reuse/a3_linear_transform_schedule_interning_plan.md)
- [A4 encoded-diagonal compatibility sharing plan](bootstrap_key_reuse/a4_encoded_diagonal_compatibility_sharing_plan.md)
- [A5 verified RNS-slice sharing plan](bootstrap_key_reuse/a5_verified_rns_slice_sharing_plan.md)
- [A6 superset-output DropLevel view plan](bootstrap_key_reuse/a6_superset_output_drop_level_plan.md)
- [Final target-level reuse spec](bootstrap_key_reuse/final_target_level_reuse_spec.md)
- [Final target-level reuse implementation plan](bootstrap_key_reuse/final_target_level_reuse_implementation_plan.md)

## Project Overview

- [Top-level README](../README.md)
- [Examples README](../examples/README.md)
- [Multiparty README](../multiparty/README.md)

## Package References

- [Ring arithmetic](../ring/README.md)
- [RLWE core](../core/rlwe/README.md)
- [RGSW blind rotations](../core/rgsw/blindrot/README.md)
- [BFV scheme](../schemes/bfv/README.md)
- [BGV scheme](../schemes/bgv/README.md)
- [CKKS scheme](../schemes/ckks/README.md)
- [CKKS bootstrapping](../circuits/ckks/bootstrapping/README.md)
- [CKKS minimax](../circuits/ckks/minimax/README.md)

## Maintenance

- Add stable docs here, then link them from this index.
- Keep `AGENTS.md` short by moving details into focused documents.
- Put reusable harness constraints in `golden-rules/`.
- Prefer concise documents with clear ownership and links to source code or
  commands that verify the claim.
