import type { User } from "@/lib/api/types";
// @ts-ignore: CSS module import for side effects
import "./avatar.css";

/** The customer's picture, or their first initial until they add one. Used everywhere the account shows. */
export default function Avatar({ user, className = "" }: { user: Pick<User, "name" | "avatar_url">; className?: string }) {
  return (
    <span className={`avatar ${className}`.trim()} aria-hidden="true">
      {user.avatar_url ? (
        <img className="avatar__img" src={user.avatar_url} alt="" />
      ) : (
        user.name.trim().charAt(0)
      )}
    </span>
  );
}
