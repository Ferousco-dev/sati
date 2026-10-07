const formatter = new Intl.NumberFormat("en-NG", {
  style: "currency",
  currency: "NGN",
  minimumFractionDigits: 0,
  maximumFractionDigits: 2,
});

/** The API sends money as whole kobo. 750000 becomes ₦7,500. */
export function formatKobo(kobo: number): string {
  return formatter.format(kobo / 100);
}
