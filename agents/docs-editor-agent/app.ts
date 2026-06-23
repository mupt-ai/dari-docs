import { flue } from '@flue/runtime/routing';
import { Hono } from 'hono';

const app = new Hono();

app.get('/healthz', (c) => c.json({ ok: true, service: 'docs-editor-agent' }));
app.route('/', flue());

export default app;
