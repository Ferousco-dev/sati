import { Suspense } from "react";
import AuthToken from "@/components/AuthToken/AuthToken";

export default function VerifyEmailPage() {
  return (
    <Suspense>
      <AuthToken mode="verify" />
    </Suspense>
  );
}
