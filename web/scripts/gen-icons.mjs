// Generates the PWA icons (regular + maskable) from a hand-drawn SVG mark.
// Run: node scripts/gen-icons.mjs  (requires sharp, a devDependency)
// Output is committed to web/public/icons/.
import sharp from 'sharp';
import { mkdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const outDir = path.join(root, 'public', 'icons');
await mkdir(outDir, { recursive: true });

const BG = '#0d1016';
const BG_RAISED = '#161b26';
const BLUE = '#3d7fff';
const BLUE_SOFT = '#6ea3ff';

// The mark: a stack of two "photo" cards with a blue mountain-and-sun motif,
// PS5-flavoured: deep charcoal, one confident blue accent, generous radii.
function mark({ maskable }) {
  // Card group occupying the central area; shrunk into the ~80% safe zone
  // for maskable variants.
  const scale = maskable ? 0.72 : 0.88;
  const t = (1 - scale) / 2 * 1024;
  const cards = `
    <g transform="translate(${t} ${t}) scale(${scale})">
      <!-- back card -->
      <g transform="rotate(-6 512 512)">
        <rect x="268" y="292" width="488" height="488" rx="56" fill="#202739" stroke="#2c3550" stroke-width="6"/>
      </g>
      <!-- front card -->
      <g transform="rotate(4 512 512)">
        <rect x="268" y="244" width="488" height="488" rx="56" fill="#1a2130" stroke="#39445f" stroke-width="8"/>
        <clipPath id="card">
          <rect x="268" y="244" width="488" height="488" rx="56"/>
        </clipPath>
        <g clip-path="url(#card)">
          <rect x="268" y="244" width="488" height="488" fill="url(#sky)"/>
          <circle cx="620" cy="392" r="54" fill="#ffffff" opacity="0.92"/>
          <path d="M268 622 L448 448 L560 566 L648 486 L756 608 L756 732 L268 732 Z" fill="#0d1016" opacity="0.55"/>
          <path d="M268 660 L392 522 L520 672 L620 580 L756 716 L756 732 L268 732 Z" fill="#0d1016" opacity="0.85"/>
        </g>
      </g>
    </g>`;
  const bg = maskable
    ? `<rect x="0" y="0" width="1024" height="1024" fill="url(#bg)"/>`
    : `<rect x="12" y="12" width="1000" height="1000" rx="224" fill="url(#bg)"/>`;
  return `
  <svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024">
    <defs>
      <linearGradient id="bg" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stop-color="${BG_RAISED}"/>
        <stop offset="1" stop-color="#10141d"/>
      </linearGradient>
      <linearGradient id="sky" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" stop-color="${BLUE}"/>
        <stop offset="1" stop-color="${BLUE_SOFT}"/>
      </linearGradient>
    </defs>
    <rect x="0" y="0" width="1024" height="1024" fill="${BG}" opacity="0"/>
    ${bg}
    ${cards}
  </svg>`;
}

async function render(name, size, maskable) {
  const svg = Buffer.from(mark({ maskable }));
  await sharp(svg).resize(size, size).png().toFile(path.join(outDir, name));
  console.log(`wrote icons/${name} (${size}x${size}${maskable ? ', maskable' : ''})`);
}

await render('icon-192.png', 192, false);
await render('icon-512.png', 512, false);
await render('maskable-192.png', 192, true);
await render('maskable-512.png', 512, true);
console.log('done');
