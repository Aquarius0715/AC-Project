---
document_id: PREP-STYLE
version: 0.7.0
status: source-inspected
owner: design-agent
updated: 2026-09-14
---

# Reference page design analysis and reasons for adoption

The reference is the [customer Loyalty page](https://aconland-mudah-milik.vercel.app/customer/loyalty). Its HTML, two linked CSS files, and part of the public JavaScript were retrieved read-only. Browser rendering, computed styles, and reward redemption have not been tested. The following maps declarations to classes found in the source; it is not a pixel-match test result.

## Evidence and review method

[Extracted values and file hashes](sources/reference-style-evidence.json) record URLs, SHA-256 hashes, CSS declarations, and HTML classes. The full HTML, which includes personal names and email addresses, was not copied into docs.

| Evidence ID | Source | Findings |
|---|---|---|
| STYLE-01 | [Font CSS](https://aconland-mudah-milik.vercel.app/_next/static/chunks/0xh6wh_1u6m3d.css) | Font-face declarations and variables for Plus Jakarta Sans, Geist Mono, and Bricolage Grotesque |
| STYLE-02 | [Shared CSS](https://aconland-mudah-milik.vercel.app/_next/static/chunks/0xipyi-ld-nvh.css) | Colors, radii, shadows, font sizes, breakpoints, and focus declarations |
| STYLE-03 | Loyalty HTML above | Applied classes, inline sidebar width variable, and heading/card/button structure |
| STYLE-04 | [Public UI chunk](https://aconland-mudah-milik.vercel.app/_next/static/chunks/0_n04r.htcg3l.js) | Mobile means below 1280px. The page's inline value overrides the default sidebar width |

## Visual style to adopt

| Item | Observed value/layout | Adoption rule |
|---|---|---|
| Primary color | #005BEA, white text, soft blue #E6F0FF | primary/primary-soft; replaces the earlier #1D4ED8 proposal |
| Page/cards | Background #F8FBFF, white surface, surface-2 #EDF6FF | Shared light-blue page and white cards across roles |
| Dark color | #0D2238 | Body text, headings, emphasis panels; use a dark panel only for the main page summary |
| Secondary text | #0D2238 at 72% / 50% | muted/subtle; adjust important labels under the exceptions below so they are not too faint |
| Body font | Plus Jakarta Sans | Latin text/numbers on all work screens; h1 inherits the same font without a separate font class |
| Supporting fonts | Geist Mono, Bricolage Grotesque | Use mono for IDs/code. Bricolage is defined but must not automatically be applied to the Loyalty h1 |
| Headings | h1 24→30px (at 768px and above), 700; h2 16px/600 | Shared typography scale |
| Cards | rounded-xl=14px, border #D6E4F5, shadow 0 14px 34px rgba(13,34,56,.055) | Do not infer defaults from class names; define tokens directly |
| Buttons/other radii | rounded-md=10px, rounded-lg=16px | Separate semantic tokens for cards and controls; do not reorder values to match size names |
| Content width/spacing | max-w-6xl=72rem, horizontal 16→32px, vertical 24→32px (at 1280px and above) | Base page width 1152px; wider dense tables need a stated reason |
| Navigation | Left sidebar 240px, collapsed 56px; top header and drawer below 1280px | Share CSS/JS boundaries; distinguish the roles of 640/768/1024/1280 |
| Information hierarchy | Heading → dark summary → KPIs → details/history | Adopt as a shared page pattern; replace points with each role's business metrics |
| KPIs | 2→4 columns (at 768px and above), gap 12→16px, value 24→30px | Explicit extension to one column when long translations or 360px width cause layout problems |
| Details/history | Main content plus 20rem side area at 1024px and above, gap 20px | Optional shared detail pattern; place history below on small screens |
| Icons | Mostly inline SVG; sidebar 16px/stroke 1.75; Lucide found in toggle | Standardize on Lucide and match size/stroke; do not claim every SVG is from Lucide |

## Parts not copied directly

- Important text at 10–11px and controls at 28px height in the reference are adjusted to at least 12px text, 14–16px main body text, and 44px touch areas. Readability and usability in work screens guide adoption.
- Combinations such as success text #059669 on light green may fall short of AA contrast targets. Adjust the semantic text token to #166534 and keep the reference value for decoration.
- Hover movement and pressed scaling may follow the reference, but stop under reduced-motion. Do not move monitoring values with decorative animation.
- Background grid/aurora styles exist in shared CSS. They may be hidden behind the Loyalty body's opaque background, so do not add them to work screens.
- Do not automatically add loyalty/points features, reference personal information, or logo assets as product requirements or redistributable assets. Use the text “AC Project” until brand images are supplied.

UX-04/08 in the [shared UIUX specification](../03-uiux/UIUXSpecification.md) define the final adopted values. Checking HTML/CSS values does not mean every site screen or production quality has been tested.
