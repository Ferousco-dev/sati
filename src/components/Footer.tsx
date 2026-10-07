// @ts-ignore: CSS module import for side effects
import "./footer.css";
import InstallButton from "@/components/InstallButton";
import FooterVisit from "@/components/FooterVisit";
import type { Footer as FooterContent, StoreInfo, Zone } from "@/lib/content/types";

type Column = { title: string; links: { label: string; href: string }[] };

function socialLinks(store: StoreInfo): { label: string; url: string }[] {
  return [
    { label: "Instagram", url: store.instagram_url },
    { label: "TikTok", url: store.tiktok_url },
    { label: "Facebook", url: store.facebook_url },
    { label: "WhatsApp", url: store.whatsapp_url },
  ].filter((s) => s.url);
}

export default function Footer({ content, store, zones }: { content: FooterContent; store: StoreInfo; zones: Zone[] }) {
  const columns: Column[] = content.columns.map((c) => ({
    title: c.title,
    links: c.links.map((l) => ({ label: l.label, href: l.url })),
  }));
  const socials = socialLinks(store);

  return (
    <footer className="footer">
      <div className="footer__inner">
        <div className="footer__top">
          <div className="footer__brand">
            <img
              src="/images/logo.png"
              alt="SATI-L Yoghurt & Confectionery"
              className="footer__logo"
            />
            {content.tagline ? <p className="footer__tagline">{content.tagline}</p> : null}
            <InstallButton />
          </div>

          <nav className="footer__cols" aria-label="Footer">
            {columns.map((col) => (
              <div key={col.title} className="footer__col">
                <h3 className="footer__col-title">{col.title}</h3>
                <ul>
                  {col.links.map((link) => (
                    <li key={link.label}>
                      <a href={link.href} className="footer__link">
                        {link.label}
                      </a>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </nav>
        </div>

        <FooterVisit store={store} zones={zones} />

        <a href="/order" className="footer__cta" data-magnetic>
          <span>Order Now</span>
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path
              d="M5 12h14M13 6l6 6-6 6"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </a>

        <div className="footer__wordmark" aria-hidden="true">
          {store.name}
        </div>

        <div className="footer__bottom">
          <span>
            © {new Date().getFullYear()} {store.name}. All rights reserved.
            {content.legal_text ? ` ${content.legal_text}` : ""}
          </span>
          <div className="footer__socials">
            {socials.map((s) => (
              <a key={s.label} href={s.url} className="footer__social" target="_blank" rel="noopener noreferrer">
                {s.label}
              </a>
            ))}
          </div>
          <div className="footer__legal">
            <a href="/privacy" className="footer__link">
              Privacy
            </a>
            <a href="/terms" className="footer__link">
              Terms
            </a>
          </div>
        </div>
      </div>
    </footer>
  );
}
