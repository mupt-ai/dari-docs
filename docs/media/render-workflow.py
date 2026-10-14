"""Render the README's illustrated workflow (not a recording).

Run: uv run --with pillow python docs/media/render-workflow.py
Uses Menlo on macOS, or DejaVu Sans Mono on Linux. No external assets.
"""

from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

HERE = Path(__file__).resolve().parent
WIDTH, HEIGHT = 1200, 640
BACKGROUND = "#0b0a09"
CARD = "#121110"
BORDER = "#2a2724"
FOREGROUND = "#eeece9"
MUTED = "#a19b94"
BRAND = "#f9b12a"

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
            ("$ dari-docs optimize . \\", FOREGROUND),
            ('    --task "Install the SDK" \\', FOREGROUND),
            ("    --harness claude-code \\", BRAND),
            ("    --llm anthropic/claude-sonnet-4-6 \\", BRAND),
            ("    --sandbox modal", BRAND),
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
            ("01  Read the quickstart", FOREGROUND),
            ("02  Run the install command", FOREGROUND),
            ("03  Try the documented import", FOREGROUND),
            ("04  Record errors and missing steps", BRAND),
            ("    No scores. Just what actually happened.", MUTED),
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
            ("Tried: install package, then import Client", FOREGROUND),
            ("Blocked: the documented import failed", BRAND),
            ("Missing: an explicit API-key setup step", BRAND),
            ("Fix: verify the import and show auth setup", FOREGROUND),
            ("→ .dari-docs/aggregate-feedback.md", MUTED),
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
            (".dari-docs/", MUTED),
            ("  aggregate-feedback.md", FOREGROUND),
            ("  updated/", BRAND),
            ("    README.md       ← proposed documentation", FOREGROUND),
            ("    CHANGELOG.md    ← summary of changes", FOREGROUND),
        ],
        "detail": [
            ("EDITOR", "Uses the aggregate feedback"),
            ("HARNESS", "Override with --editor-harness"),
            ("MODEL", "Override with --editor-llm"),
            ("APPLY", "Manual review, or opt in: --apply"),
        ],
    },
]


def render(phase, tick):
    image = Image.new("RGB", (WIDTH, HEIGHT), BACKGROUND)
    draw = ImageDraw.Draw(image)
    draw.rectangle((0, 0, WIDTH, 4), fill=BRAND)
    draw.text((44, 31), "DARI DOCS", font=font(18), fill=FOREGROUND)
    draw.text(
        (1156, 34),
        "HOW OPTIMIZE WORKS / ILLUSTRATED",
        font=font(12),
        fill=MUTED,
        anchor="ra",
    )
    draw.line((44, 75, 1156, 75), fill=BORDER)
    data = PHASES[phase]
    draw.text((44, 102), data["name"], font=font(34), fill=FOREGROUND)
    draw.text((46, 154), data["subtitle"], font=font(16), fill=MUTED)

    labels = ["Task", "Tester", "Feedback", "Editor"]
    for index, label in enumerate(labels):
        x = 44 + index * 284
        active = index == phase
        draw.rectangle(
            (x, 211, x + 260, 268), fill=CARD, outline=BRAND if active else BORDER
        )
        draw.text(
            (x + 17, 231),
            f"0{index + 1}",
            font=font(12),
            fill=BRAND if active else MUTED,
        )
        draw.text(
            (x + 57, 228), label, font=font(17), fill=FOREGROUND if active else MUTED
        )
        if index < 3:
            draw.text((x + 267, 231), ">", font=font(12), fill=MUTED)

    draw.rectangle((44, 301, 703, 555), fill=CARD, outline=BORDER)
    draw.text((65, 322), data["label"], font=font(11), fill=BRAND)
    draw.line((65, 350, 681, 350), fill=BORDER)
    for index, (text, color) in enumerate(data["lines"]):
        draw.text((65, 375 + index * 32), text, font=font(13), fill=color)

    draw.rectangle((727, 301, 1156, 555), fill=CARD, outline=BORDER)
    draw.text(
        (750, 322),
        "RUN CONFIGURATION" if phase == 0 else "WHAT YOU GET",
        font=font(11),
        fill=MUTED,
    )
    for index, (key, value) in enumerate(data["detail"]):
        y = 367 + index * 44
        draw.text((750, y), key, font=font(9), fill=MUTED)
        draw.text((750, y + 14), value, font=font(12), fill=FOREGROUND)

    draw.line((44, 585, 1156, 585), fill=BORDER)
    footer = (
        "* Public URL sources support check only."
        if phase == 0
        else "Self-deployed on your keys. Harbor adapters determine compatibility."
    )
    draw.text((44, 605), footer, font=font(10), fill=MUTED)
    for index in range(4):
        x = 1057 + index * 25
        draw.rectangle((x, 606, x + 14, 609), fill=BRAND if index == phase else BORDER)
    # Small progress rail adds movement without flashing or hiding any text.
    start = 44 + phase * 278
    draw.rectangle((start, 584, start + int(278 * tick), 586), fill=BRAND)
    return image


def main():
    frames = [render(phase, tick / 4) for phase in range(4) for tick in range(1, 5)]
    frames[0].save(
        HERE / "docs-workflow.gif",
        save_all=True,
        append_images=frames[1:],
        duration=800,
        loop=0,
        optimize=True,
        disposal=1,
    )
    print(f"Rendered {len(frames)} frames: {HERE / 'docs-workflow.gif'}")


if __name__ == "__main__":
    main()
