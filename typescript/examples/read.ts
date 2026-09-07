import { HeyClient } from "../src/index.js";
const hey = new HeyClient({ token: process.env.HEY_TOKEN! });
for await (const page of hey.pages("ListContacts", {})) console.log(page.data);
