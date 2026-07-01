# FT-020 Verification Summary

Local verification complete. Prod verification (CHK-04) pending operator deploy (AG-01).

| Check | Covers | Result | Evidence |
| --- | --- | --- | --- |
| CHK-01 | EC-01/03, SC-01/02 | PASS — biz+data+task -race all ok | artifacts/ft-020/verify/chk-01/go-test-race.log |
| CHK-02 | EC-03 | PASS — all FT-020 files lint-clean (residual = pre-existing non-FT-020 files) | artifacts/ft-020/verify/chk-02/{make-lint.log,lint-analysis.txt} |
| CHK-03 | REQ-01, CON-02 | PASS — make test green; goose applies 3 FT-020 migrations to fresh+seeded PG | artifacts/ft-020/verify/chk-03/{make-test.log,migration-applies.txt} |
| CHK-04 | EC-02/04, SC-01 | PENDING — requires prod deploy (AG-01) then curl /v1/feed | artifacts/ft-020/verify/chk-04/ (empty) |
| CHK-05 | EC-04, MET-02 | PASS — SKILL.md untouched | artifacts/ft-020/verify/chk-05/skill-unchanged.txt |

## New FT-020 tests (acceptance)
- biz: TestScorePost_DigestBranch (SC-01/EC-01); TestScoreUsecase_EnqueueDue sums posts+digests (REQ-01); interest Create/Update/Delete assert EnqueueScoredDigests (REQ-06/FM-03)
- data: TestIntegration_ScoreRepo_UpsertBySummaryID (CTR-01); _ListRanked_IncludesDigest (SC-01/CTR-03); _ListRanked_MixedKeysetPagination (SC-02); PostEmbeddingRepo_DigestKeyRoundTrip (REQ-03); ScoreOutbox_EnqueueDueDigests (CTR-04/REQ-01)
- service: feed_test asserts kind mapping POST/DIGEST (CTR-02)

## Simplify review (STEP-17): PASS
Diff mirrors existing patterns (partner enqueue methods, shared embeddingKeyColumn/scanSummaryIDs helpers, two-branch upserts justified by CTR-01 distinct ON CONFLICT targets). ListRanked SQL extracted to a package const to satisfy funlen without logic change.
