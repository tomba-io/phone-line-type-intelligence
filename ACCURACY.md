# North American Carrier Accuracy

## What this measures, and what it doesn't

Two different questions get conflated as "accuracy":

1. **Does the table faithfully reflect published assignment data?** — Near-perfect.
   Deterministic joins over authoritative files. Errors here are bugs.
2. **Does the table reflect the number's _current_ line type?** — Systematically
   wrong on every ported number, and this cannot be fixed within Layer 1.

Only the second matters to your customers. Do not quote the first as your
accuracy figure.

## The error is one-directional

Two decades of cord-cutting mean wireline-to-wireless porting dominates the
reverse. So the characteristic failure is:

> Number allocated to an ILEC block in 1998, ported to a mobile carrier in
> 2011 — this table says `wireline`, you drop a reachable mobile number.

**False negatives on mobile, not false positives.** That is the good direction
for a data product: you under-return rather than return wrong. Every miss is
inventory you don't bill for, not a customer complaint.

## Do not trust anyone's published ported-number percentage

Including ours. Aggregate porting statistics don't transfer to your corpus,
because your error rate depends on things specific to you:

- **Age of the numbers.** Long-held numbers have more porting history.
- **Geography.** Pooled metro rate centres behave differently from rural NXXs.
- **Institutional vs personal.** A university main line has likely never
  ported; a personal cell may have ported multiple times.

Your corpus is unique. Measure it.

## Measurement protocol

1. **Sample.** Freeze a random sample of 2,000-5,000 records from production
   traffic. Stratify by NPA so dense metros don't dominate.
2. **Classify.** Run through the linetype package.
3. **Verify.** Dip the same numbers through a paid line-type API (Telnyx,
   Bandwidth). Cost at this sample size is negligible.
4. **Confusion matrix.** The cell that matters is **predicted wireline →
   actual wireless**: that's your leak rate.
5. **Reverse check.** Also check predicted wireless → actual wireline.

## OCN-to-Line-Type Mapping

NANPA and CNAC do not publish line type. The classification is derived from the
Operating Company Number (OCN) category:

| OCN Category            | Line Type | Rationale                                        |
| ----------------------- | --------- | ------------------------------------------------ |
| ILEC, RBOC              | wireline  | Incumbent local exchange carriers                |
| CLEC, CAP               | voip      | CLECs are predominantly VoIP/SMS-reachable today |
| WIRELESS, PCS, W RESLLR | wireless  | Mobile carriers and resellers                    |
| IPES, LRESLLR           | voip      | IP-enabled and local resellers                   |
| PAGING                  | paging    | Paging services                                  |

Mexico's IFT publishes the service modality directly (`CPP`/`MPP` = mobile,
`FIJO` = fixed), so no OCN mapping is needed.

## Classification accuracy by type

| Line Type    | Assignment Accuracy | Porting Error Direction                                     |
| ------------ | :-----------------: | ----------------------------------------------------------- |
| **wireless** |        ~98%         | Slight over-count: some wireless numbers ported to wireline |
| **wireline** |        ~86%         | Under-count: ported-out numbers still show as wireline      |
| **voip**     |        ~97%         | CLEC classification captures most VoIP                      |
| **tollfree** |        ~100%        | Toll-free numbers are not portable between carrier types    |
| **unknown**  |         N/A         | Coverage gap, not a classification error                    |

## Layer 1 vs Layer 2

|              | Layer 1 (this package)             | Layer 2 (paid API)                       |
| ------------ | ---------------------------------- | ---------------------------------------- |
| **Source**   | Published allocation data          | NPAC porting database                    |
| **Cost**     | Zero (embedded)                    | Per-lookup fee                           |
| **Latency**  | 12 ns                              | 50-200 ms                                |
| **Porting**  | Not reflected                      | Reflected                                |
| **Best for** | Bulk classification, pre-filtering | Individual verification, TCPA compliance |

**Recommended architecture:** Use Layer 1 for all numbers. Send only the
`wireline` and `unknown` results to Layer 2 for verification. This cuts your
paid API volume by 55-65%.

## Claims you can and cannot make

**Can say:** "assignment-derived line type", "based on NANPA published allocation
data", "identifies the carrier type a number range was allocated to".

**Cannot say:** anything framed as TCPA compliance, Do-Not-Call safety, or
"verified wireless". Those require NPAC-derived porting data available only to
registered entities.
