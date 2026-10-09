import { Suspense } from "react";
import AuthToken from "@/components/AuthToken/AuthToken";

export default function ResetPasswordPage() {
  return (
    <Suspense>
      <AuthToken mode="reset" />
    </Suspense>
  );
}
