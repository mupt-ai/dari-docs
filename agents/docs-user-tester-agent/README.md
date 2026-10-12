# Documentation tester

The shared Python `harbor_runner.py` loads `prompts/system.md`, stages the supplied docs, and runs a Harbor trial. The agent must write `/app/output/feedback.md`; the controller validates it and returns `{"result":{"feedback":"..."}}`.

Local development, from the parent agents directory:

```bash
python harbor_runner.py --service tester --port 8000
```

Select a harness/model/sandbox in the request's `execution` settings or use `dari-docs --harness pi --sandbox modal --llm anthropic/claude-sonnet-4-6`. Credentials belong in the controller environment or Modal secret, not in prompts or saved config. See the root docs for setup and progress APIs.

The prompt and skill files remain editable. The prompt is always included; the skill content is also included in the task instructions so custom harnesses need not support Harbor's native skill registration.
