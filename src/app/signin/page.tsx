import { Suspense } from "react";
import AuthForm from "@/components/AuthForm/AuthForm";

export default function SignInPage() {
  return (
    <Suspense>
      <AuthForm mode="signin" />
    </Suspense>
  );
}
