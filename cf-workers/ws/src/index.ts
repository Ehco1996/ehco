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
	const queryParams = url.searchParams;
	console.log('Request URL:', url.href, url.searchParams);
	for (const [key, value] of queryParams) {
		console.log(`${key}: ${value}`);
	}
	const webSocketPair = new WebSocketPair();
	const [client, server] = Object.values(webSocketPair);
	server.accept();

	const address = { hostname: '127.0.0.1', port: 5201 };
	const tcpSocket = connect(address);

	const readableStream = new ReadableStream({
		start(controller) {
			server.addEventListener('message', (event) => {
				controller.enqueue(event.data);
			});
			server.addEventListener('close', () => {
				controller.close();
				client.close();
				server.close();
				tcpSocket.close();
			});
			server.addEventListener('error', (err) => {
				controller.error(err);
				client.close();
				server.close();
				tcpSocket.close();
			});
		},
	});

	const writableStream = new WritableStream({
		write(chunk) {
			server.send(chunk);
		},
		close() {
			client.close();
			server.close();
			tcpSocket.close();
		},
		abort(err) {
			console.error('Stream error:', err);
			client.close();
			server.close();
			tcpSocket.close();
		},
	});

	readableStream
		.pipeTo(tcpSocket.writable)
		.then(() => console.log('All data successfully written!'))
		.catch((e) => {
			console.error('Something went wrong on read!', e.message);
			client.close();
			server.close();
			tcpSocket.close();
		});

	tcpSocket.readable
		.pipeTo(writableStream)
		.then(() => console.log('All data successfully written!'))
		.catch((e) => {
			console.error('Something went wrong on write!', e.message);
			client.close();
			server.close();
			tcpSocket.close();
		});

	return new Response(null, {
		status: 101,
		webSocket: client,
	});
}

export default {
	async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
		return handleRequest(request);
	},
};
