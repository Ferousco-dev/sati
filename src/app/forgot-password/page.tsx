import { Suspense } from "react";
import AuthToken from "@/components/AuthToken/AuthToken";

export default function ForgotPasswordPage() {
  return (
    <Suspense>
      <AuthToken mode="forgot" />
    </Suspense>
  );
}
