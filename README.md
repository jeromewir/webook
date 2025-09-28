## Wework auto-book

Allows you to host a small webserver that listen to request and make booking for a desk for a specific date at a specific location

### How to find the required WeWork location ID ?

- Navigate to the [booking page](https://members.wework.com/workplaceone/content2/bookings/desks)
- Copy the following text and replace `115 Broadway` with the exact name of the coworking

```
document.evaluate(`//div[text()="115 Broadway"]`, document).iterateNext().parentNode.parentNode.parentNode.parentNode.parentNode.parentNode.id
```

- Open dev console and paste the updated code
- Copy the result and paste it in the env file
