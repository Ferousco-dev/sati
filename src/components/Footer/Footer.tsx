// @ts-ignore: CSS module import for side effects
import "./footer.css";
import InstallButton from "@/components/InstallButton/InstallButton";
import { fallbackLogo } from "@/lib/fallbacks";
import type { Footer as FooterContent, Navigation, StoreInfo } from "@/lib/content/types";
import Img from "@/components/ui/Img";

const SOCIALS = ["Instagram", "TikTok", "WhatsApp", "Facebook"] as const;

function socialUrl(name: (typeof SOCIALS)[number], store: StoreInfo): string {
  const urls = {
    Instagram: store.instagram_url,
    TikTok: store.tiktok_url,
    WhatsApp: store.whatsapp_url,
    Facebook: store.facebook_url,
  };
  return urls[name] || "#";
}

export default function Footer({
  content,
  store,
  navigation,
}: {
  content: FooterContent;
  store: StoreInfo;
  navigation: Navigation;
}) {
  const columns = content.columns.map((c) => ({
    title: c.title,
    links: c.links.map((l) => ({ label: l.label, href: l.url })),
  }));

  // The store address and phone from the admin settings lead the Visit column, above its other links.
  const contact = [
    store.address && {
      label: store.address,
      href: `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(store.address)}`,
    },
    store.phone && { label: store.phone, href: `tel:${store.phone.replace(/\s+/g, "")}` },
  ].filter(Boolean) as { label: string; href: string }[];
  if (contact.length) {
    const visit = columns.find((c) => c.title === "Visit");
    if (visit) visit.links = [...contact, ...visit.links];
    else columns.push({ title: "Visit", links: contact });
  }
  return (
    <footer className="footer">
      <div className="footer__inner">
        <div className="footer__top">
          <div className="footer__brand">
            <Img
              src={navigation.logo?.url ?? fallbackLogo}
              alt={navigation.logo_alt}
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

        {/* <a href={content.cta_url || "/menu"} className="footer__cta" data-magnetic>
          <span>{content.cta_label}</span>
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path
              d="M5 12h14M13 6l6 6-6 6"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </a> */}
        <div className="footer__bottom">
          <span>
            © {new Date().getFullYear()} {store.name}. All rights reserved.
            {content.legal_text ? ` ${content.legal_text}` : ""}
          </span>
          <div className="footer__socials">
            {SOCIALS.map((name) => (
              <a key={name} href={socialUrl(name, store)} className="footer__social">
                {name}
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

        <div className="footer__wordmark" aria-hidden="true">
          {store.name}
        </div>
    </footer>
  );
}
