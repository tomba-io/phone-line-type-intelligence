# Data Sources

Every URL below was checked live. `get-data.sh` fetches all of them; this file
exists so you can grab one by hand, or find the index page when a direct link
moves.

**Geographic blocking:** `reports.nanpa.com` sits behind Imperva and returns
HTTP 403 to several countries outright. Copy `.proxyrc.example` to `.proxyrc`
and fill it in — the script sources it automatically.

---

## United States — NANPA

NXX-level and thousands-block assignments. **Does not publish line type.**

| File                        | Direct link                                                                     |     Size |
| --------------------------- | ------------------------------------------------------------------------------- | -------: |
| CO codes, all states        | https://reports.nanpa.com/public/CoCodeAssignment_Utilized_AllStates_Public.zip |  2.19 MB |
| Thousands blocks, augmented | https://reports.nanpa.com/public/ThousandsBlockAssignment_All_Augmented.zip     | 12.28 MB |

Use the **augmented** block file. The plain one omits the code-holder columns.

Index pages, if a direct link ever 404s:

- CO codes — https://www.nanpa.com/reports/co-code-reports/cocodes_assign
- Thousands blocks — https://www.nanpa.com/reports/thousands-block-reports/region

---

## Canada — CNAC

Same two levels as NANPA, different column names and a spelled-out status
vocabulary. **Does not publish line type.**

| File                     | Direct link                                |   Size |
| ------------------------ | ------------------------------------------ | -----: |
| CO code status, all NPAs | https://cnac.ca/data/COCodeStatus_ALL.zip  | 337 KB |
| Block status, all NPAs   | https://cnac.ca/data/COBlockStatus_ALL.zip | 3.2 KB |

---

## Mexico — IFT

Plan Nacional de Numeración. **Publishes the service type directly**
(`CPP`/`MPP` = mobile, `FIJO` = fixed), so Mexico needs no OCN map.

There is **no direct link**. IFT serves it from a JSF form:

- https://sns.ift.org.mx/sns-frontend/planes-numeracion/descarga-publica.xhtml

Click _Descargar_. `get-data.sh` automates the ViewState POST.

---

## What none of these give you

**Porting.** Every source above is allocation data. A number ported from its
original carrier still reports that carrier. Fixing it requires **NPAC**
(Number Portability Administration Center), which is restricted to registered
Service Providers.
