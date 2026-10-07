// @ts-ignore: CSS module import for side effects
import "./floating.css";

/** A floating chat button. It only appears once the store has set a WhatsApp number. */
export default function WhatsAppButton({ number }: { number: string }) {
  if (!number) return null;
  const href = `https://wa.me/${number}?text=${encodeURIComponent("Hello SATI-L, ")}`;

  return (
    <a
      className="wa-fab"
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      aria-label="Chat with us on WhatsApp"
    >
      <svg viewBox="0 0 32 32" fill="currentColor" aria-hidden="true">
        <path d="M16.04 3C9.4 3 4 8.38 4 15.02c0 2.12.55 4.19 1.6 6.01L4 29l8.15-1.56a12.03 12.03 0 0 0 3.89.64h.01C22.7 28.08 28 22.7 28 16.06 28 9.4 22.68 3 16.04 3zm0 22.04a9.98 9.98 0 0 1-3.4-.6l-.5-.17-4.83.93.96-4.7-.2-.5a9.9 9.9 0 0 1-1.5-5.2c0-5.5 4.5-10 10.03-10 5.52 0 9.9 4.47 9.9 9.98 0 5.52-4.5 10.26-10.46 10.26zm5.5-7.4c-.3-.15-1.78-.88-2.06-.98-.28-.1-.48-.15-.68.15-.2.3-.78.98-.95 1.18-.18.2-.35.22-.65.07-.3-.15-1.27-.47-2.42-1.5-.9-.8-1.5-1.78-1.67-2.08-.18-.3-.02-.46.13-.6.13-.14.3-.35.45-.52.15-.18.2-.3.3-.5.1-.2.05-.38-.03-.53-.07-.15-.68-1.64-.93-2.25-.25-.58-.5-.5-.68-.5h-.58c-.2 0-.53.07-.8.38-.28.3-1.05 1.03-1.05 2.5 0 1.48 1.08 2.9 1.23 3.1.15.2 2.13 3.25 5.15 4.55.72.3 1.28.5 1.72.64.72.23 1.38.2 1.9.12.58-.09 1.78-.73 2.03-1.43.25-.7.25-1.3.18-1.43-.08-.13-.28-.2-.58-.35z" />
      </svg>
    </a>
  );
}
