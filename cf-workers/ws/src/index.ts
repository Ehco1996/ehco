/**
 * Welcome to Cloudflare Workers! This is your first worker.
 *
 * - Run `npm run dev` in your terminal to start a development server
 * - Open a browser tab at http://localhost:8787/ to see your worker in action
 * - Run `npm run deploy` to publish your worker
 *
 * Bind resources to your worker in `wrangler.toml`. After adding bindings, a type definition for the
 * `Env` object can be regenerated with `npm run cf-typegen`.
 *
 * Learn more at https://developers.cloudflare.com/workers/
 */
import { connect } from 'cloudflare:sockets';

async function handleRequest(request) {
	const upgradeHeader = request.headers.get('Upgrade');
	if (!upgradeHeader || upgradeHeader !== 'websocket') {
		return new Response('Expected Upgrade: websocket', { status: 426 });
	}

	const url = new URL(request.url);
	console.log('Request URL:', url.href);
	const queryParams = url.searchParams;
	for (const [key, value] of queryParams) {
		console.log(`${key}: ${value}`);
	}
	const webSocketPair = new WebSocketPair();
	const [client, server] = Object.values(webSocketPair);
	server.accept();

	const readableStream = new ReadableStream({
		start(controller) {
			server.onmessage = (event) => {
				controller.enqueue(event.data);
			};
			server.onclose = () => {
				controller.close();
			};
			server.onerror = (err) => {
				controller.error(err);
			};
		},
	});

	const writableStream = new WritableStream({
		write(chunk) {
			server.send(chunk);
		},
		close() {
			server.close();
		},
		abort(err) {
			console.error('Stream error:', err);
			server.close();
		},
	});

	const address = { hostname: '127.0.0.1', port: 5201 };
	const tcpSocket = connect(address);
	readableStream.pipeTo(tcpSocket.writable);
	tcpSocket.readable.pipeTo(writableStream);
	return new Response(null, {
		status: 101,
		webSocket: client,
	});
}

addEventListener('fetch', (event) => {
	event.respondWith(handleRequest(event.request));
});

export default {
	async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
		return handleRequest(request);
	},
};
