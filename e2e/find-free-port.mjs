// Prints one free TCP port to stdout, then exits. Used by `make e2e` to
// pick a port before starting Playwright, so playwright.config.ts's static
// baseURL and global-setup.ts's server spawn agree on the same value
// without either having to await an async port lookup at module-load time.
import { createServer } from 'node:net';

const server = createServer();
server.listen(0, '127.0.0.1', () => {
    const { port } = server.address();
    server.close(() => {
        process.stdout.write(String(port));
    });
});
