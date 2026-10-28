// bun entry point for the Android build: WSL on Windows, bash elsewhere.
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const script = fileURLToPath(new URL('./build-apk', import.meta.url));
const [command, args] =
  process.platform === 'win32'
    ? ['wsl.exe', ['-d', process.env.PARLEY_WSL_DISTRO ?? 'arch', '--', 'bash', script
        .replace(/^([A-Za-z]):\\/, (_, drive) => `/mnt/${drive.toLowerCase()}/`)
        .replaceAll('\\', '/')]]
    : ['bash', [script]];

const result = spawnSync(command, args, { stdio: 'inherit' });
process.exit(result.status ?? 1);
