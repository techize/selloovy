# Go dependency notices

These unmodified licence texts cover module dependencies used by the application, worker or migration command at this increment. They are upstream notices, not merchant or customer records. Test-only dependencies and CI tools are recorded separately as they are introduced.

| Module | Version | Licence | Notice | SHA-256 |
| --- | --- | --- | --- | --- |
| dario.cat/mergo | v1.0.2 | BSD-3-Clause | [dario_cat_mergo.LICENSE](dario_cat_mergo.LICENSE) | cb7684632b729955293cab8a0bbf4134d53d9532a30761c33c8a4878302a7bd3 |
| github.com/Masterminds/goutils | v1.1.1 | Apache-2.0 | [github_com_Masterminds_goutils.LICENSE.txt](github_com_Masterminds_goutils.LICENSE.txt) | cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30 |
| github.com/Masterminds/semver/v3 | v3.5.0 | MIT | [github_com_Masterminds_semver_v3.LICENSE.txt](github_com_Masterminds_semver_v3.LICENSE.txt) | 2db9097f7b3750c1c9426996d21fe443d339131fb758c2167d2d5fabe8e6cbf9 |
| github.com/Masterminds/sprig/v3 | v3.3.0 | MIT | [github_com_Masterminds_sprig_v3.LICENSE.txt](github_com_Masterminds_sprig_v3.LICENSE.txt) | dbad9a1f3fee465db42b92cb8a41ddaae028248fdcc54f46f1e72f7707d21dc2 |
| github.com/go-chi/chi/v5 | v5.3.2 | MIT | [chi.LICENSE](chi.LICENSE) | a2d51b7515acfaff2f7a88688650f2fc4fd99561383e72bba2305e3db59a1647 |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause | [github_com_google_uuid.LICENSE](github_com_google_uuid.LICENSE) | 0a8d61ed3cbfd5312326e8126c31ce9c627a283adc99131b56896d29ada04b2d |
| github.com/huandu/xstrings | v1.5.0 | MIT | [github_com_huandu_xstrings.LICENSE](github_com_huandu_xstrings.LICENSE) | d298ca9d57c94e16547637f1d7122481ecdb83932c9b901fac349c4c382b82eb |
| github.com/jackc/pgpassfile | v1.0.0 | MIT | [github_com_jackc_pgpassfile.LICENSE](github_com_jackc_pgpassfile.LICENSE) | adb1663fda031df8f4344aa68f299fd87d80353e31339406742ded21dae65702 |
| github.com/jackc/pgservicefile | v0.0.0-20240606120523-5a60cdf6a761 | MIT | [github_com_jackc_pgservicefile.LICENSE](github_com_jackc_pgservicefile.LICENSE) | fc505773403fe869ed64cc2235cdd13988a427bb7e3a7e7004a3f4b27420f8fc |
| github.com/jackc/pgx/v5 | v5.11.0 | MIT | [github_com_jackc_pgx_v5.LICENSE](github_com_jackc_pgx_v5.LICENSE) | 467f95e074fe23079a5623ed652619682692041b8551da27e3c2ddb9659a1507 |
| github.com/jackc/puddle/v2 | v2.2.2 | MIT | [github_com_jackc_puddle_v2.LICENSE](github_com_jackc_puddle_v2.LICENSE) | 2d50e98a4900b4d6457a38d39c1432fdc156fc2f7b365f2e33ec9344acbb0057 |
| github.com/jackc/tern/v2 | v2.4.3 | MIT | [github_com_jackc_tern_v2.LICENSE](github_com_jackc_tern_v2.LICENSE) | 1ea0a00bef36541487cf48912bd5c2e019a2fa02028819543c212ce8d4b68feb |
| github.com/mitchellh/copystructure | v1.2.0 | MIT | [github_com_mitchellh_copystructure.LICENSE](github_com_mitchellh_copystructure.LICENSE) | 3c377fad2e5ae1d7081c7c2f16da867a87cca1d1f5f1aa7ed0b8a16bb553142a |
| github.com/mitchellh/reflectwalk | v1.0.2 | MIT | [github_com_mitchellh_reflectwalk.LICENSE](github_com_mitchellh_reflectwalk.LICENSE) | 22adc4abdece712a737573672f082fd61ac2b21df878efb87ffcff4354a07f26 |
| github.com/shopspring/decimal | v1.4.0 | MIT | [github_com_shopspring_decimal.LICENSE](github_com_shopspring_decimal.LICENSE) | b92ba0f6ee02f2309628bfdadb123668a17c016e475ba477b857d33470d9d625 |
| github.com/spf13/cast | v1.10.0 | MIT | [github_com_spf13_cast.LICENSE](github_com_spf13_cast.LICENSE) | feb6d17a0e7a64e5ab0f7e2b0e0ee3e69c1a6396626fd554dc0cddaa06851b44 |
| golang.org/x/crypto | v0.55.0 | BSD-3-Clause | [golang_org_x_crypto.LICENSE](golang_org_x_crypto.LICENSE) | 911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad |
| golang.org/x/sync | v0.22.0 | BSD-3-Clause | [golang_org_x_sync.LICENSE](golang_org_x_sync.LICENSE) | 911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad |
| golang.org/x/sys | v0.47.0 | BSD-3-Clause | [golang_org_x_sys.LICENSE](golang_org_x_sys.LICENSE) | 911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad |
| golang.org/x/text | v0.41.0 | BSD-3-Clause | [golang_org_x_text.LICENSE](golang_org_x_text.LICENSE) | 911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad |

Development generator: sqlc v1.31.1, MIT, [sqlc.LICENSE](sqlc.LICENSE), SHA-256 fc69a4525f6f7e79675fba3645351f2cdaf9331978e72d44155634575a4455ea. [Upstream release](https://github.com/sqlc-dev/sqlc/releases/tag/v1.31.1). The tool/dependency tree is not bundled; generated query source compiles with the existing pgx runtime.
