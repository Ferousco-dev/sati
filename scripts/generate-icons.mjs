// Draws the placeholder app icons: a white S on the brand blue.
// Replace the files in public/icons and src/app (icon.png, apple-icon.png) with the real logo when it is ready.
import sharp from "sharp";
import { mkdir } from "node:fs/promises";

const BLUE = "#268adc";
const S_PATH =
  "M 67 31 C 66 22 58 18 50 18 C 40 18 34 24 34 31 C 34 41 45 43 52 46 C 61 49 68 53 68 63 C 68 72 60 78 50 78 C 41 78 34 73 33 65";

function svg({ size, radius, scale }) {
  return Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 100 100">
  <rect width="100" height="100" rx="${radius}" fill="${BLUE}"/>
  <g transform="translate(50 50) scale(${scale}) translate(-50 -50)">
    <path d="${S_PATH}" fill="none" stroke="#ffffff" stroke-width="13" stroke-linecap="round" stroke-linejoin="round"/>
  </g>
</svg>`);
}

const icons = [
  { file: "public/icons/icon-192.png", size: 192, radius: 22, scale: 1 },
  { file: "public/icons/icon-512.png", size: 512, radius: 22, scale: 1 },
  // Maskable icons are cropped by the phone, so the letter stays inside the safe centre.
  { file: "public/icons/icon-maskable-512.png", size: 512, radius: 0, scale: 0.72 },
  { file: "public/icons/apple-touch-icon.png", size: 180, radius: 0, scale: 0.85 },
  { file: "src/app/icon.png", size: 512, radius: 22, scale: 1 },
  { file: "src/app/apple-icon.png", size: 180, radius: 0, scale: 0.85 },
];

await mkdir("public/icons", { recursive: true });
for (const { file, ...opts } of icons) {
  await sharp(svg(opts)).png().toFile(file);
  console.log("wrote", file);
}
