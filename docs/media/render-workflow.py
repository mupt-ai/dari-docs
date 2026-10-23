"""Render the README's illustrated workflow (not a recording).

Run: uv run --with pillow python docs/media/render-workflow.py
Writes docs-workflow-light.gif and docs-workflow-dark.gif, using the selfbench.dev
palettes (warm paper and ink blue). Uses Menlo on macOS, or DejaVu Sans Mono on Linux.
No external assets.
"""

from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

HERE = Path(__file__).resolve().parent
WIDTH, HEIGHT = 1200, 640

# Tokens from selfbench's review/src/public-site/palette.css, converted to hex.
THEMES = {
    "light": {
        "background": "#f5f3ef",
        "card": "#eeebe6",
        "border": "#ddd7d0",
        "foreground": "#211f1c",
        "muted": "#67615b",
        "accent": "#307e57",
    },
    "dark": {
        "background": "#090c10",
        "card": "#10151b",
        "border": "#22282f",
        "foreground": "#d4dbe2",
        "muted": "#89939f",
        "accent": "#45a173",
    },
}

FONT_PATH = next(
    path
    for path in [
        Path("/System/Library/Fonts/Menlo.ttc"),
        Path("/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"),
    ]
    if path.exists()
)


def font(size):
    return ImageFont.truetype(str(FONT_PATH), size)


PHASES = [
    {
        "name": "Choose a task",
        "subtitle": "What should a developer be able to do?",
        "label": "YOUR INPUT",
        "lines": [
            ("$ dari-docs optimize . \\", "foreground"),
            ('    --task "Install the SDK" \\', "foreground"),
            ("    --harness claude-code \\", "accent"),
            ("    --llm anthropic/claude-sonnet-4-6 \\", "accent"),
            ("    --sandbox modal", "accent"),
        ],
        "detail": [
            ("SOURCE", "Selected docs or a public URL*"),
            ("HARNESS", "Claude Code / Codex / Pi / OpenCode"),
            ("MODEL", "Your provider + model"),
            ("WORKSPACE", "A fresh sandbox"),
        ],
    },
    {
        "name": "Let the tester try",
        "subtitle": "The agent follows your docs, not a checklist.",
        "label": "ILLUSTRATIVE ATTEMPT",
        "lines": [
            ("01  Read the quickstart", "foreground"),
            ("02  Run the install command", "foreground"),
            ("03  Try the documented import", "foreground"),
            ("04  Record errors and missing steps", "accent"),
            ("    No scores. Just what actually happened.", "muted"),
        ],
        "detail": [
            ("BOUNDARY", "Isolated task workspace"),
            ("METHOD", "Read / run / inspect"),
            ("CREDENTIALS", "Your deployment's provider keys"),
            ("LIVE APIs", "Product credentials are opt-in"),
        ],
    },
    {
        "name": "See the blockers",
        "subtitle": "Get evidence and the smallest useful docs fixes.",
        "label": "ILLUSTRATIVE FEEDBACK",
        "lines": [
            ("Tried: install package, then import Client", "foreground"),
            ("Blocked: the documented import failed", "accent"),
            ("Missing: an explicit API-key setup step", "accent"),
            ("Fix: verify the import and show auth setup", "foreground"),
            ("→ .dari-docs/aggregate-feedback.md", "muted"),
        ],
        "detail": [
            ("REPORT", "Attempts / outcomes / blockers"),
            ("BATCHING", "Multiple tasks and models"),
            ("PARALLELISM", "--parallel N"),
            ("NEXT", "Aggregate findings for the editor"),
        ],
    },
    {
        "name": "Review the proposed edits",
        "subtitle": "Optimize proposes changes. You decide what ships.",
        "label": "FILES TO REVIEW",
        "lines": [
            (".dari-docs/", "muted"),
            ("  aggregate-feedback.md", "foreground"),
            ("  updated/", "accent"),
            ("    README.md       ← proposed documentation", "foreground"),
            ("    CHANGELOG.md    ← summary of changes", "foreground"),
        ],
        "detail": [
            ("EDITOR", "Uses the aggregate feedback"),
            ("HARNESS", "Override with --editor-harness"),
            ("MODEL", "Override with --editor-llm"),
            ("APPLY", "Manual review, or opt in: --apply"),
        ],
    },
]


def render(theme, phase, tick):
    c = THEMES[theme]
    image = Image.new("RGB", (WIDTH, HEIGHT), c["background"])
    draw = ImageDraw.Draw(image)
    draw.text((44, 31), "DARI DOCS", font=font(18), fill=c["foreground"])
    draw.text(
        (1156, 34),
        "HOW OPTIMIZE WORKS / ILLUSTRATED",
        font=font(12),
        fill=c["muted"],
        anchor="ra",
    )
    draw.line((44, 75, 1156, 75), fill=c["border"])
    data = PHASES[phase]
    draw.text((44, 102), data["name"], font=font(34), fill=c["foreground"])
    draw.text((46, 154), data["subtitle"], font=font(16), fill=c["muted"])

    labels = ["Task", "Tester", "Feedback", "Editor"]
    for index, label in enumerate(labels):
        x = 44 + index * 284
        active = index == phase
        draw.rectangle(
            (x, 211, x + 260, 268),
            fill=c["card"],
            outline=c["accent"] if active else c["border"],
        )
        draw.text(
            (x + 17, 231),
            f"0{index + 1}",
            font=font(12),
            fill=c["accent"] if active else c["muted"],
        )
        draw.text(
            (x + 57, 228),
            label,
            font=font(17),
            fill=c["foreground"] if active else c["muted"],
        )
        if index < 3:
            draw.text((x + 267, 231), ">", font=font(12), fill=c["muted"])

    draw.rectangle((44, 301, 703, 555), fill=c["card"], outline=c["border"])
    draw.text((65, 322), data["label"], font=font(11), fill=c["accent"])
    draw.line((65, 350, 681, 350), fill=c["border"])
    for index, (text, role) in enumerate(data["lines"]):
        draw.text((65, 375 + index * 32), text, font=font(13), fill=c[role])

    draw.rectangle((727, 301, 1156, 555), fill=c["card"], outline=c["border"])
    draw.text(
        (750, 322),
        "RUN CONFIGURATION" if phase == 0 else "WHAT YOU GET",
        font=font(11),
        fill=c["muted"],
    )
    for index, (key, value) in enumerate(data["detail"]):
        y = 367 + index * 44
        draw.text((750, y), key, font=font(9), fill=c["muted"])
        draw.text((750, y + 14), value, font=font(12), fill=c["foreground"])

    draw.line((44, 585, 1156, 585), fill=c["border"])
    footer = (
        "* Public URL sources support check only."
        if phase == 0
        else "Self-deployed on your keys. Harbor adapters determine compatibility."
    )
    draw.text((44, 605), footer, font=font(10), fill=c["muted"])
    for index in range(4):
        x = 1057 + index * 25
        draw.rectangle(
            (x, 606, x + 14, 609),
            fill=c["accent"] if index == phase else c["border"],
        )
    # Small progress rail adds movement without flashing or hiding any text.
    start = 44 + phase * 278
    draw.rectangle((start, 584, start + int(278 * tick), 586), fill=c["accent"])
    return image


def main():
    for theme in THEMES:
        frames = [
            render(theme, phase, tick / 4) for phase in range(4) for tick in range(1, 5)
        ]
        output = HERE / f"docs-workflow-{theme}.gif"
        frames[0].save(
            output,
            save_all=True,
            append_images=frames[1:],
            duration=800,
            loop=0,
            optimize=True,
            disposal=1,
        )
        print(f"Rendered {len(frames)} frames: {output}")


if __name__ == "__main__":
    main()
