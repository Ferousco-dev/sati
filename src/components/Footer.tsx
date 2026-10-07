// @ts-ignore: CSS module import for side effects
import "./footer.css";
import InstallButton from "@/components/InstallButton";

const columns = [
  {
    title: "Menu",
    links: [
      { label: "Parfait", href: "/menu#parfait" },
      { label: "Desserts & Cakes", href: "/menu#cakes" },
      { label: "Small Chops", href: "/menu#small-chops" },
      { label: "Yoghurt", href: "/menu#yoghurt" },
      { label: "Healthy Drinks", href: "/menu#drinks" },
    ],
  },
  {
    title: "SATI-L",
    links: [
      { label: "Our Story", href: "/brand" },
      { label: "Find a Spot", href: "/stores" },
      { label: "Catering", href: "/catering" },
      { label: "Careers", href: "/careers" },
    ],
  },
  {
    title: "Visit",
    links: [
      { label: "Visit our store", href: "/stores" },
      { label: "Open 9am – 11pm daily", href: "/stores" },
      {
        label: "hello@orentegrills.com",
        href: "mailto:hello@orentegrills.com",
      },
    ],
  },
];

const socials = ["Instagram", "TikTok", "WhatsApp", "Facebook"];

export default function Footer() {
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
            <p className="footer__tagline">Make you ask for more</p>
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
          SATI-L Yoghurt & confectionery 
        </div>

        <div className="footer__bottom">
          <span>© {2026} SATI-L Yoghurt & confectionery . All rights reserved.</span>
          <div className="footer__socials">
            {socials.map((s) => (
              <a key={s} href="#" className="footer__social">
                {s}
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
