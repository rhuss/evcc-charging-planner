# Charging Planner for evcc

[evcc](https://evcc.io) is a powerful tool for flexibly charging your electric vehicles. However, one feature that's currently missing is a weekly charging plan to ensure your car is charged and ready by a configured time each day. 

While evcc allows you to set a single target time for the end of a charging session, this setting is cleared after the session ends, requiring manual re-entry for regular schedules. 

Fortunately, thanks to evcc's extensibility, you can implement such a scheduler yourself. `evcc-charging-planner` is a proof of concept that implements such a planner.

Here's how it works:

- **Event Detection**: When evcc detects a vehicle connection, it can trigger an event to external services as configured in its settings.
- **Charging Plan Calculation**: An external program listens for these events and calculates the next charging session's end time. By default, evcc uses excess solar energy from your PV system, but it ensures your car reaches the configured State of Charge (SoC) by the specified time. If solar energy isn't sufficient, it draws the remaining energy from the grid, potentially optimizing costs by using dynamic electricity pricing.
- **Plan Submission**: The program sends the calculated end time and target SoC to evcc, which sets the charging schedule accordingly.

In summary, this setup automates the process of configuring a charging plan in evcc whenever your car is connected to the wallbox.

> [!IMPORTANT]
> This project is a Proof of Concept, meaning long-term support is not planned.  
> It requires the latest version of evcc, which you must compile from the [evcc GitHub repository](https://github.com/evcc-io/evcc). There is no official release yet.

## Example Configuration

```yaml
# MQTT configuration to connect to the broker. You can use the same 
# configuration options as evcc itself. 
# See https://docs.evcc.io/docs/reference/configuration/mqtt
mqtt:
  broker: "broker.mqtt"
  user: "user"
  password: "pass"
  # Topics for connecting to the evcc instance. The following 
  # values are the defaults and usually don't need to be changed.
  topics:
    # Topic to listen on for connect/disconnect events
    events: "evcc/events"
    # Topic to send the target charging time. 
    # Use %s as a placeholder for the vehicle ID
    planSoc: "evcc/vehicles/%s/planSoc"

# List of vehicles to monitor for connection events    
vehicles:
  - name: "car"
    # Default SoC when not specified in individual entries
    soc: 80
    # Weekly schedule for charging
    schedule:
      # Day can be any weekday or special keywords like "weekend" (Sat & Sun) 
      # or "workday" (Mon - Fri)
      - day: "Monday"
        # Time when charging must be complete, with the target SoC reached
        time: "07:00"
        # Override default SoC
        soc: 70
      - day: "Wednesday"
        time: "07:00"
        # Uses the global SoC (80) as no specific value is provided
      - day: "Friday"
        time: "07:00"
        soc: 90
      - day: "workday"
        time: "08:00"
        # Applies to all weekdays not explicitly listed
      - day: "weekend"
        time: "09:00"
        soc: 60
```
   
### Communication via MQTT
To enable this functionality, set up MQTT integration in evcc with the following configuration:

``` yaml
# Connect to the same MQTT broker as evcc-charging-planner
mqtt:
  broker: "broker.mqtt"
  user: "user"
  password: "pass"

# Push messages
messaging:
  events:
    connect:
      # ${vehicleName} is replaced by the vehicle ID, and ${mode} by the charging mode (e.g., "PV").
      # The message type must be "connect".
      # Keep the message format as-is, as evcc-charging-planner expects this structure.
      msg: '{"vehicle": "${vehicleName}", "mode": "${mode}", "type": "connect"}'
  services:
    # New "custom" service for sending push messages.
    # Any plugin described in https://docs.evcc.io/docs/reference/plugins can be used.
    - type: custom
      send:
        # Triggers MQTT plugins
        source: mqtt
        # Send all push events to this topic (default value)
        topic: "evcc/events"
```

### Future Development

In the future, this functionality could be integrated directly into evcc. Other potential enhancements include:

* **Google Calendar Integration**: Sync with calendar events (e.g., business trips) to set charging end times based on scheduled meetings.
* **Natural Language Scheduling**: Use AI to allow users to define charging plans with natural language commands.
* **Weather and Tariff Forecasts**: Incorporate weather and dynamic pricing data to optimize charging costs.

### Feedback

For questions or suggestions, please open an issue on GitHub.
