# Page Render-Info Fixture (Story 11-6)

`render-info.pdf` is a hand-authored, uncompressed PDF 1.5 that exercises every
branch of the `dump page --info` assembled view: geometry inheritance,
ExtGState blend/alpha/SMask, Form vs Image XObject classification, an ICCBased
colorspace, a transparency group, structural-only Pattern/Shading entries,
nested forms, and a deliberately self-referential form.

It is committed as a `.pdf` (not generated at test time) so the failure mode
stays explicit and reviewable, matching `testdata/correctness/`. Both the
pdfcore unit tests (`internal/pdfcore/page_render_test.go`) and the CLI
black-box tests (`tests/cli-page-render/`) consume it. Tests that need a fresh
in-memory variant (e.g. the no-`/Resources` empty-array path) build their own
bytes inline; this committed file is the rich end-to-end happy-path fixture.

All fixtures use a PDF 1.5 header, a plain `xref` table (no XRefStream),
uncompressed content streams (no Flate/LZW), and no encryption.

## Object layout (14 objects)

```
obj 1  Catalog                /Pages 2 0 R
obj 2  Pages                  /Kids [3 0 R] /Count 1 /Rotate 90 /MediaBox [0 0 300 400]
                              -- MediaBox + Rotate live HERE, inherited by the page (AC1 gotcha)
obj 3  Page                   /Parent 2 0 R /Contents 4 0 R
                              /Resources <<
                                /ExtGState << /GS0 5 0 R >>
                                /XObject   << /Fm0 8 0 R /FmSelf 10 0 R /Im0 11 0 R >>
                                /Pattern   << /P0 13 0 R >>
                                /Shading   << /Sh0 14 0 R >>
                              >>
                              -- NO MediaBox / Rotate on the page: they resolve via inheritance
obj 4  page content stream    "q /GS0 gs /Fm0 Do Q  q /Im0 Do Q"
obj 5  ExtGState GS0          /BM /Multiply /ca 0.5 /CA 1.0 /SMask 6 0 R          (AC2)
obj 6  SMask dict             /S /Luminosity /G 7 0 R /BC [0]                      (AC2: resolved descriptor)
obj 7  Form (SMask /G group)  /Group << /S /Transparency /CS /DeviceGray /I true /K false >>
obj 8  Form Fm0               /BBox [0 0 100 100] /Matrix [1 0 0 1 10 10]
                              /Group << /S /Transparency /CS /DeviceRGB /I false /K true >>
                              /Resources << /XObject << /Fm1 9 0 R >> >>           (AC4: form's OWN resources)
                              content "q /Fm1 Do Q"
obj 9  Form Fm1               /BBox [0 0 50 50]  content "0 0 50 50 re f"          (leaf nested form)
obj 10 Form FmSelf            /BBox [0 0 20 20]
                              /Resources << /XObject << /FmSelf 10 0 R >> >>       (AC4: SELF-REFERENTIAL)
                              content "q /FmSelf Do Q"
obj 11 Image Im0              /Width 4 /Height 4 /BitsPerComponent 8
                              /ColorSpace [/ICCBased 12 0 R]                       (AC3: ICCBased)
                              content = 48 raw bytes (4x4x3, no filter)
obj 12 ICC profile stream     /N 3 /Alternate /DeviceRGB                          (30-byte placeholder profile)
obj 13 Pattern P0             /PatternType 2 /Shading 14 0 R                       (AC1/AC7: structural only)
obj 14 Shading Sh0            /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 1 1]
                              /Function << /FunctionType 2 /Domain [0 1] /C0 [0 0 0] /C1 [1 1 1] /N 1 >>
```

## Regeneration

The PDF is small enough to edit by hand, but to regenerate it byte-for-byte:
write each object body above as `N 0 obj\n<body>\nendobj\n\n`, recording each
object's start offset; emit a `xref` table with those offsets (object 0 is the
`0000000000 65535 f ` free head); then a
`trailer << /Size 15 /Root 1 0 R >>`, `startxref <xref-offset>`, `%%EOF`.
Stream objects are `<dict>\nstream\n<content>endstream`; the dict's `/Length`
must equal the content byte count exactly (no trailing newline inside the
stream beyond what `/Length` counts). pdfcpu validates required entries, so the
shading carries a `/Function` and MediaBox is present (on `/Pages`) for the
validator.

## Expected `dump page --info 1 --forms-recursive` highlights

- `mediaBox: [0,0,300,400]`, `rotate: 90` -- both inherited from obj 2.
- `extGStates[0]`: `BM:"Multiply"`, `ca:0.5`, `CA:1`, and the resolved soft-mask
  descriptor emitted INLINE as the SMask value:
  `SMask:{S:"Luminosity", gRef:"7 0 R", bcSize:1}`.
- `xobjects`: `Fm0` (Form, bbox/matrix/group), `FmSelf` (Form), `Im0`
  (Image, `colorSpace:{family:"ICCBased", n:3, iccProfileSize:30, altFamily:"DeviceRGB"}`).
- `patterns[0].patternType: 2`, `shadings[0].shadingType: 2`.
- `forms`: `Fm0` resolved against its own resources -> `Fm1`; `FmSelf` ->
  `FmSelf` marked `cyclic: true` (the self-referential walk terminates).
