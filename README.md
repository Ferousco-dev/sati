# SATI Yoghurt

SATI Yoghurt is a digital platform for ordering SATI Yoghurt products online, with support for pickup, delivery, payments, and order tracking.

This root only holds this README and the ignore rules. Each part of the product is its own branch of the same remote, cloned into a folder of the same name.

| Folder | Branch | What it is |
| --- | --- | --- |
| `frontend` | `frontend` | The Next.js website for customers and the admin dashboard. |
| `backend` | `backend` | The Go API for authentication, products, orders, payments, delivery, notifications, and store operations. |

## Setup

```bash
git clone --branch main --single-branch https://github.com/Ferousco-dev/sati.git sati
cd sati

git clone --branch frontend --single-branch https://github.com/Ferousco-dev/sati.git frontend
git clone --branch backend --single-branch https://github.com/Ferousco-dev/sati.git backend
```

Then follow the README inside each folder. Run git commands, installs, tests, and commits inside the folder that owns the change, never from this root.

