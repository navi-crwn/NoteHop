import { readFile, readdir, stat } from 'node:fs/promises';
import { join, dirname, resolve, extname } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));
const docs = join(root, 'docs');

const htmlFiles = [
  'index.html',
  'en/index.html',
  'id/index.html',
  'zh/index.html',
  'es/index.html',
  'ar/index.html',
  'pt/index.html',
  'fr/index.html',
  'ru/index.html',
  'ja/index.html'
];

let errors = 0;

for (const relHtml of htmlFiles) {
  const filePath = join(docs, relHtml);
  let content;
  try {
    content = await readFile(filePath, 'utf8');
  } catch (err) {
    console.error(`Missing HTML file: ${relHtml}`);
    errors++;
    continue;
  }

  // Find all ids in this document
  const ids = new Set();
  const idRegex = /\sid=["']([^"']+)["']/g;
  let idMatch;
  while ((idMatch = idRegex.exec(content)) !== null) {
    ids.add(idMatch[1]);
  }

  // Find all hrefs and srcs
  const refRegex = /(?:href|src)=["']([^"']+)["']/g;
  let refMatch;
  while ((refMatch = refRegex.exec(content)) !== null) {
    const rawRef = refMatch[1].trim();
    if (!rawRef || rawRef.startsWith('mailto:') || rawRef.startsWith('tel:') || rawRef.startsWith('javascript:')) {
      continue;
    }
    if (rawRef.startsWith('http://') || rawRef.startsWith('https://')) {
      // External URL - ensure valid URL
      try {
        new URL(rawRef);
      } catch (e) {
        console.error(`[${relHtml}] Invalid URL: ${rawRef}`);
        errors++;
      }
      continue;
    }

    // Relative reference
    const [pathPart, hashPart] = rawRef.split('#');
    let targetFile = filePath;
    if (pathPart) {
      targetFile = resolve(dirname(filePath), pathPart);
      if (pathPart.endsWith('/') || !extname(pathPart)) {
        targetFile = join(targetFile, 'index.html');
      }
      try {
        await stat(targetFile);
      } catch {
        console.error(`[${relHtml}] Broken link: ${rawRef} -> target does not exist: ${targetFile}`);
        errors++;
        continue;
      }
    }

    if (hashPart) {
      if (pathPart && targetFile !== filePath) {
        // Read target file and check id
        const targetContent = await readFile(targetFile, 'utf8');
        if (!targetContent.includes(`id="${hashPart}"`) && !targetContent.includes(`id='${hashPart}'`)) {
          console.error(`[${relHtml}] Missing target hash: #${hashPart} in ${targetFile}`);
          errors++;
        }
      } else {
        if (!ids.has(hashPart)) {
          console.error(`[${relHtml}] Missing target hash: #${hashPart}`);
          errors++;
        }
      }
    }
  }
}

if (errors === 0) {
  console.log(`All ${htmlFiles.length} pages validated! 0 broken links.`);
  process.exit(0);
} else {
  console.error(`Validation failed with ${errors} errors.`);
  process.exit(1);
}
