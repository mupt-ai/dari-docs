import { flue } from '@flue/runtime/routing';
import { Hono } from 'hono';

const app = new Hono();

app.get('/healthz', (c) => c.json({ ok: true, service: 'docs-user-tester-agent' }));
app.route('/', flue());

export default app;
