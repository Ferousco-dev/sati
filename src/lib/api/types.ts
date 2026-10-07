export type Role = "CUSTOMER" | "STAFF" | "ADMIN" | "RIDER";

export type User = {
  id: string;
  name: string;
  email: string;
  role: Role;
  email_verified: boolean;
  created_at: string;
};

export type ApiErrorBody = {
  error: { code: string; message: string };
};
