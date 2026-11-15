import { defineConfig } from 'vite';
import { createHash } from 'node:crypto';
import { readdirSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const here = path.dirname(fileURLToPath(import.meta.url));
const assets = path.resolve(here, '../internal/webui/assets');
function sourceDigest() {
  const hash = createHash('sha256');
  function walk(directory, prefix = '') {
    for (const entry of readdirSync(directory, { withFileTypes: true }).sort((a,b) => a.name < b.name ? -1 : a.name > b.name ? 1 : 0)) {
      const name = prefix + entry.name;
      if (entry.isDirectory()) walk(path.join(directory,entry.name),name+'/');
      else if (entry.isFile()) hash.update(name+'\0').update(readFileSync(path.join(directory,entry.name))).update('\0');
    }
  }
  walk(assets);
  return hash.digest('hex');
}

export default defineConfig({
  base: '/',
  build: { outDir: '../internal/webui/optimized/dist', emptyOutDir: true, sourcemap: false },
  plugins: [{ name: 'core-source-digest', generateBundle() {
    this.emitFile({ type: 'asset', fileName: 'source.sha256', source: sourceDigest()+'\n' });
  }}]
});
