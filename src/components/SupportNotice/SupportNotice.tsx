"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api/client";

/** Tells a customer how to ask for a deleted account back, using the store's own contact details. */
export default function SupportNotice() {
  const [email, setEmail] = useState("");

  useEffect(() => {
    api
      .get<{ store: { email: string } }>("/api/v1/store")
      .then((r) => setEmail(r.store.email), () => {});
  }, []);

  return (
    <p className="auth__ok">
      To use this account again, please email our support team
      {email ? (
        <>
          {" "}
          at{" "}
          <a className="auth__link" href={`mailto:${email}?subject=Restore%20my%20account`}>
            {email}
          </a>
        </>
      ) : null}
      . Deleted accounts cannot be restored from the website.
    </p>
  );
}
